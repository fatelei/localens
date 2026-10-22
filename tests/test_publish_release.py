"""Publication recovery must preserve releases and reject untrusted build sources."""
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location('publish_release', Path(__file__).resolve().parents[1] / 'scripts/publish_release.py')
publish = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(publish)


class PublishTests(unittest.TestCase):
    def test_existing_release_uploads_assets_without_recreating_metadata(self):
        with patch.object(publish, 'verify_assets', return_value=[Path('dist/a.zip'), Path('dist/SHA256SUMS')]), patch.object(publish, 'github_json', return_value=[[{'tag_name': 'v1.0.0'}]]), patch.object(publish.subprocess, 'run') as run:
            publish.publish(Path('dist'), 'v1.0.0', 'owner/repo')
        command = run.call_args.args[0]
        self.assertEqual(command[:4], ['gh', 'release', 'upload', 'v1.0.0'])
        self.assertNotIn('--clobber', command)
        self.assertNotIn('create', command)

    def test_new_prerelease_is_created(self):
        with patch.object(publish, 'verify_assets', return_value=[Path('dist/a.zip')]), patch.object(publish, 'github_json', return_value=[[]]), patch.object(publish.subprocess, 'run') as run:
            publish.publish(Path('dist'), 'v1.0.1-beta.1', 'owner/repo')
        command = run.call_args.args[0]
        self.assertEqual(command[:4], ['gh', 'release', 'create', 'v1.0.1-beta.1'])
        self.assertIn('--verify-tag', command)
        self.assertIn('--prerelease', command)

    def test_api_failure_never_triggers_release_creation(self):
        with patch.object(publish, 'verify_assets', return_value=[]), patch.object(publish, 'github_json', side_effect=RuntimeError('API unavailable')), patch.object(publish.subprocess, 'run') as run:
            with self.assertRaises(RuntimeError):
                publish.publish(Path('dist'), 'v1.0.0', 'owner/repo')
            run.assert_not_called()

    def make_assets(self, root):
        for system, arch in publish.TARGETS:
            name = f'Shiwa-v1.0.0-{system}-{arch}-full' + ('.tar.gz' if system == 'linux' else '.zip')
            path = root / name
            path.write_bytes(name.encode())
            (root / (name + '.sha256')).write_text(f'{publish.digest(path)}  {name}\n')
            (root / (name + '.smoke.json')).write_text(json.dumps({
                'version': 'v1.0.0', 'model_revision': publish.MODEL_REVISION,
                'system': system, 'arch': arch, 'offline': True, 'query': True,
                'text': True, 'image': True, 'socket_access': 'denied', 'dimensions': 256,
            }))

    def test_identical_existing_assets_are_skipped_and_different_assets_rejected(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            self.make_assets(root)
            files = publish.verify_assets(root, 'v1.0.0')
            assets = [{'name': p.name, 'digest': 'sha256:' + publish.digest(p)} for p in files]
            pages = [[{'tag_name': 'v1.0.0', 'assets': assets}]]
            with patch.object(publish, 'github_json', return_value=pages), patch.object(publish.subprocess, 'run') as run:
                publish.publish(root, 'v1.0.0', 'owner/repo')
                run.assert_not_called()
                assets[0]['digest'] = 'sha256:wrong'
                with self.assertRaisesRegex(ValueError, 'Existing asset differs'):
                    publish.publish(root, 'v1.0.0', 'owner/repo')
                run.assert_not_called()

    def test_validation_requires_all_packages_checksums_and_exact_target_reports(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            self.make_assets(root)
            files = publish.verify_assets(root, 'v1.0.0')
            self.assertEqual(len(files), 11)
            report = files[1]
            data = json.loads(report.read_text())
            data['arch'] = 'wrong'
            report.write_text(json.dumps(data))
            with self.assertRaisesRegex(ValueError, 'target'):
                publish.verify_assets(root, 'v1.0.0')
            self.make_assets(root)
            files[0].write_bytes(b'corrupt')
            with self.assertRaisesRegex(ValueError, 'Checksum'):
                publish.verify_assets(root, 'v1.0.0')
            files[0].unlink()
            with self.assertRaisesRegex(ValueError, 'Incomplete'):
                publish.verify_assets(root, 'v1.0.0')

    def test_recovery_requires_matching_tag_and_all_native_checks(self):
        source = {'status': 'completed', 'event': 'push', 'path': '.github/workflows/release.yml', 'head_branch': 'v1.0.0', 'head_sha': 'abc', 'head_repository': {'full_name': 'owner/repo'}}
        jobs = {'jobs': [{'name': name, 'conclusion': 'success'} for name in publish.REQUIRED_JOBS]}
        with patch.object(publish, 'github_json', side_effect=[source, {'sha': 'abc'}, jobs]):
            publish.validate_source('owner/repo', 'v1.0.0', '123')
        with patch.object(publish, 'github_json', side_effect=[source, {'sha': 'changed'}]):
            with self.assertRaises(ValueError):
                publish.validate_source('owner/repo', 'v1.0.0', '123')
        jobs['jobs'][0]['conclusion'] = 'failure'
        with patch.object(publish, 'github_json', side_effect=[source, {'sha': 'abc'}, jobs]):
            with self.assertRaises(ValueError):
                publish.validate_source('owner/repo', 'v1.0.0', '123')


if __name__ == '__main__':
    unittest.main()
