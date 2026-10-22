#!/usr/bin/env python3
"""Validate complete release assets and publish safely, including retries."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess

TARGETS = (('windows', 'amd64'), ('windows', 'arm64'), ('linux', 'amd64'), ('linux', 'arm64'), ('macos', 'arm64'))
REQUIRED_JOBS = ('checks / lint', *(f'checks / {system}-{arch}' for system, arch in TARGETS))
MODEL_REVISION = '914f7f89142e33e77833254d9c9b90c3cef7303b'


def validate_tag(tag):
    if not re.fullmatch(r'v\d+\.\d+\.\d+(?:-[0-9A-Za-z][0-9A-Za-z.-]*)?(?:\+[0-9A-Za-z.-]+)?', tag):
        raise ValueError('Invalid release tag')


def github_json(path, paginate=False):
    command = ['gh', 'api']
    if paginate:
        command += ['--paginate', '--slurp']
    return json.loads(subprocess.check_output([*command, path], text=True))


def validate_source(repo, tag, run_id):
    validate_tag(tag)
    if not str(run_id).isdigit() or int(run_id) <= 0:
        raise ValueError('Invalid source run ID')
    source = github_json(f'repos/{repo}/actions/runs/{run_id}')
    if (source.get('status') != 'completed' or source.get('event') != 'push'
            or source.get('path', '').split('@')[0] != '.github/workflows/release.yml'
            or source.get('head_branch') != tag
            or source.get('head_repository', {}).get('full_name') != repo):
        raise ValueError('Source must be a completed tag Release run in this repository')
    commit = github_json(f'repos/{repo}/commits/{tag}')
    if source.get('head_sha') != commit.get('sha'):
        raise ValueError('Source run does not match the current release tag commit')
    jobs = github_json(f'repos/{repo}/actions/runs/{run_id}/jobs?filter=latest&per_page=100')
    results = {job['name']: job.get('conclusion') for job in jobs['jobs']}
    if any(results.get(name) != 'success' for name in REQUIRED_JOBS):
        raise ValueError('Source run did not pass all five native package jobs and lint')


def digest(path):
    h = hashlib.sha256()
    with path.open('rb') as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b''):
            h.update(block)
    return h.hexdigest()


def verify_assets(root, tag):
    validate_tag(tag)
    expected = {f'Shiwa-{tag}-{system}-{arch}-full' + ('.tar.gz' if system == 'linux' else '.zip'): (system, arch)
                for system, arch in TARGETS}
    actual = {p.name for p in root.iterdir() if p.name.endswith(('.zip', '.tar.gz'))}
    if actual != set(expected):
        raise ValueError('Incomplete release: expected exactly five complete packages')
    lines, files = [], []
    for name, (system, arch) in sorted(expected.items()):
        path = root / name
        line = f'{digest(path)}  {name}\n'
        if (root / (name + '.sha256')).read_text() != line:
            raise ValueError(f'Checksum mismatch: {name}')
        report_path = root / (name + '.smoke.json')
        report = json.loads(report_path.read_text())
        if not all(report.get(key) is True for key in ('offline', 'query', 'text', 'image')) or report.get('socket_access') != 'denied' or report.get('dimensions') != 256:
            raise ValueError(f'Missing offline inference proof: {name}')
        if report.get('version') != tag or report.get('model_revision') != MODEL_REVISION or report.get('system') != system or report.get('arch') != arch:
            raise ValueError(f'Unexpected model/version/target: {name}')
        lines.append(line)
        files += [path, report_path]
    checksums = root / 'SHA256SUMS'
    checksums.write_text(''.join(lines))
    return [*files, checksums]


def publish(root, tag, repo):
    files = verify_assets(root, tag)
    pages = github_json(f'repos/{repo}/releases?per_page=100', paginate=True)
    existing = next((release for page in pages for release in page if release['tag_name'] == tag), None)
    if existing is not None:
        assets = {asset['name']: asset for asset in existing.get('assets', [])}
        missing = []
        for path in files:
            if path.name not in assets:
                missing.append(str(path))
            elif assets[path.name].get('digest') != 'sha256:' + digest(path):
                raise ValueError(f'Existing asset differs or cannot be verified: {path.name}; use a new version')
        if missing:
            subprocess.run(['gh', 'release', 'upload', tag, *missing, '--repo', repo], check=True)
    else:
        flags = ['--prerelease'] if '-' in tag.split('+')[0] else []
        subprocess.run(['gh', 'release', 'create', tag, *map(str, files), '--repo', repo,
                        '--verify-tag', '--generate-notes', '--title', f'拾物 {tag}', *flags], check=True)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--tag', required=True)
    parser.add_argument('--repo', default=os.environ.get('GH_REPO'), required=not os.environ.get('GH_REPO'))
    parser.add_argument('--dist', type=Path, default=Path('dist'))
    parser.add_argument('--source-run')
    parser.add_argument('--check-source-run')
    args = parser.parse_args()
    if args.check_source_run or args.source_run:
        validate_source(args.repo, args.tag, args.check_source_run or args.source_run)
    if not args.check_source_run:
        publish(args.dist, args.tag, args.repo)
