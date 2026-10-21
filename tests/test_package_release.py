"""Release guards use synthetic binaries, never local documents or models."""
import importlib.util
import os
import json
from pathlib import Path
import struct
import tempfile
import unittest
from unittest.mock import patch
import tarfile

SPEC = importlib.util.spec_from_file_location('package_release', Path(__file__).resolve().parents[1] / 'scripts/package_release.py')
release = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(release)


def header(system, arch):
    data = bytearray(128)
    if system == 'windows':
        data[:2] = b'MZ'
        struct.pack_into('<I', data, 60, 64)
        data[64:68] = b'PE\0\0'
        struct.pack_into('<H', data, 68, 0x8664 if arch == 'amd64' else 0xAA64)
    elif system == 'linux':
        data[:6] = b'\x7fELF\x02\x01'
        struct.pack_into('<H', data, 18, 62 if arch == 'amd64' else 183)
    else:
        data[:4] = b'\xcf\xfa\xed\xfe'
        struct.pack_into('<I', data, 4, 0x01000007 if arch == 'amd64' else 0x0100000C)
    return data


def assets(root, system, arch):
    prepared = root / 'prepared'
    python = prepared / 'python' / ('python.exe' if system == 'windows' else 'bin/python3')
    python.parent.mkdir(parents=True)
    python.write_bytes(header(system, arch))
    python.chmod(0o755)
    (prepared / 'model').mkdir()
    for name in ('model.safetensors', 'config.json', 'tokenizer.json', 'modules.json'):
        (prepared / 'model' / name).write_bytes(b'fixture')
    (prepared / 'model-manifest.json').write_text(json.dumps({
        'id': 'google/embeddinggemma-2', 'offline': True,
        'revision': '914f7f89142e33e77833254d9c9b90c3cef7303b'}))
    (prepared / 'THIRD-PARTY-NOTICES.txt').write_text('Credits')
    return prepared


REPORT = {'offline': True, 'query': True, 'text': True, 'image': True, 'socket_access': 'denied', 'dimensions': 256}


class PackagingTests(unittest.TestCase):
    def test_architecture_guard(self):
        with tempfile.TemporaryDirectory() as temp:
            binary = Path(temp) / 'binary'
            for system in ('windows', 'linux', 'macos'):
                for arch in ('amd64', 'arm64'):
                    with self.subTest(system=system, arch=arch):
                        binary.write_bytes(header(system, arch))
                        release.verify_binary(binary, system, arch)
                        with self.assertRaises(ValueError):
                            release.verify_binary(binary, system, 'arm64' if arch == 'amd64' else 'amd64')
            binary.write_bytes(b'broken')
            with self.assertRaises(ValueError):
                release.verify_binary(binary, 'linux', 'amd64')

    def test_version_cannot_escape_output_directory(self):
        for value in ('../../file', 'v1.0.0/extra', '$(touch x)', ''):
            with self.assertRaises(ValueError):
                release.validate_version(value)
        for value in ('v1.2.3', 'v1.2.3-rc.1', 'v1.2.3+build.1', 'dev-123abcd'):
            self.assertEqual(release.validate_version(value), value)

    def test_macos_archive_excludes_stale_resources(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            source = root / 'build/bin/Shiwa.app'
            for relative, data in {
                'Contents/MacOS/Shiwa': header('macos', 'arm64'),
                'Contents/Info.plist': b'<?xml version="1.0"?><plist version="1.0"><dict/></plist>',
                'Contents/Resources/AppIcon.icns': b'icon',
                'Contents/Resources/model/private': b'excluded',
            }.items():
                path = source / relative
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_bytes(data)
            (root / 'docs').mkdir()
            (root / 'engine').mkdir()
            (root / 'docs/CI-RELEASE.md').write_text('Guide')
            (root / 'engine/THIRD-PARTY-NOTICES.txt').write_text('Credits')
            prepared = assets(root, 'macos', 'arm64')
            with patch.object(release, 'ROOT', root), patch.object(release.sys, 'platform', 'darwin'), patch.object(release, 'sign_macos'), patch.object(release, 'smoke_resources', return_value=REPORT.copy()):
                archive = release.package('v1.2.3', 'arm64', root / 'dist', prepared)
            import zipfile
            with zipfile.ZipFile(archive) as bundle:
                self.assertFalse(any('private' in name for name in bundle.namelist()))
                self.assertIn('Shiwa/拾物.app/Contents/Resources/model/model.safetensors', bundle.namelist())
                self.assertIn('Shiwa/拾物.app/Contents/MacOS/Shiwa', bundle.namelist())

    def test_linux_archive_contains_offline_assets_and_checksums(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            for directory in ('build/bin', 'docs', 'engine', '.models'):
                (root / directory).mkdir(parents=True)
            binary = root / 'build/bin/Shiwa'
            binary.write_bytes(header('linux', 'arm64'))
            binary.chmod(0o755)
            (root / 'docs/CI-RELEASE.md').write_text('Guide')
            (root / 'engine/THIRD-PARTY-NOTICES.txt').write_text('Credits')
            (root / '.models/private').write_text('must not ship')
            prepared = assets(root, 'linux', 'arm64')
            with patch.object(release, 'ROOT', root), patch.object(release.sys, 'platform', 'linux'), patch.object(release, 'smoke_resources', return_value=REPORT.copy()):
                archive = release.package('v1.2.3', 'arm64', root / 'dist', prepared)
            with tarfile.open(archive) as bundle:
                self.assertIn('Shiwa/Resources/model/model.safetensors', bundle.getnames())
                self.assertIn('Shiwa/Resources/python/bin/python3', bundle.getnames())
                self.assertFalse(any('private' in name for name in bundle.getnames()))
                if os.name != 'nt':
                    self.assertTrue(bundle.getmember('Shiwa/Shiwa').mode & 0o111)
            self.assertTrue(json.loads(archive.with_name(archive.name + '.smoke.json').read_text())['offline'])
            self.assertIn('-full.tar.gz', archive.name)
            self.assertEqual(archive.with_name(archive.name + '.sha256').read_text(), f'{release.digest(archive)}  {archive.name}\n')


    def test_missing_runtime_or_wrong_architecture_cannot_package(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            with patch.object(release.sys, 'platform', 'linux'):
                with self.assertRaises(ValueError):
                    release.package('v1.2.3', 'arm64', root / 'dist', root / 'missing')
            prepared = assets(root, 'linux', 'amd64')
            with self.assertRaises(ValueError):
                release.validate_assets(prepared, 'linux', 'arm64')
            self.assertFalse((root / 'dist').exists())

    def test_macos_intel_release_is_rejected(self):
        with tempfile.TemporaryDirectory() as temp, patch.object(release.sys, 'platform', 'darwin'):
            with self.assertRaisesRegex(ValueError, 'Intel'):
                release.package('v1.2.3', 'amd64', Path(temp) / 'dist')

    def test_windows_archive_contains_native_runtime(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            for directory in ('build/bin', 'docs'):
                (root / directory).mkdir(parents=True)
            (root / 'build/bin/Shiwa.exe').write_bytes(header('windows', 'arm64'))
            (root / 'docs/CI-RELEASE.md').write_text('Guide')
            prepared = assets(root, 'windows', 'arm64')
            with patch.object(release, 'ROOT', root), patch.object(release.sys, 'platform', 'win32'), patch.object(release, 'smoke_resources', return_value=REPORT.copy()):
                archive = release.package('v1.2.3', 'arm64', root / 'dist', prepared)
            import zipfile
            with zipfile.ZipFile(archive) as bundle:
                self.assertIn('Shiwa/Resources/python/python.exe', bundle.namelist())
                self.assertIn('Shiwa/Resources/model/model.safetensors', bundle.namelist())

    def test_failed_inference_does_not_create_archive(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            for directory in ('build/bin', 'docs'):
                (root / directory).mkdir(parents=True)
            (root / 'build/bin/Shiwa').write_bytes(header('linux', 'arm64'))
            (root / 'docs/CI-RELEASE.md').write_text('Guide')
            prepared = assets(root, 'linux', 'arm64')
            with patch.object(release, 'ROOT', root), patch.object(release.sys, 'platform', 'linux'), patch.object(release, 'smoke_resources', side_effect=ValueError('inference failed')):
                with self.assertRaisesRegex(ValueError, 'inference failed'):
                    release.package('v1.2.3', 'arm64', root / 'dist', prepared)
            self.assertFalse(list((root / 'dist').glob('*')))


if __name__ == '__main__':
    unittest.main()
