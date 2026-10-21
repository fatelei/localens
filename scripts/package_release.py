#!/usr/bin/env python3
"""Package a complete native offline application; missing model/runtime is an error."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import plistlib
import re
import shutil
import struct
import subprocess
import sys
import tarfile
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[1]
SYSTEMS = {'darwin': 'macos', 'win32': 'windows', 'linux': 'linux'}


def validate_version(value):
    if not re.fullmatch(r'(?:v\d+\.\d+\.\d+(?:-[0-9A-Za-z][0-9A-Za-z.-]*)?(?:\+[0-9A-Za-z.-]+)?|dev-[0-9a-f]{7,40})', value):
        raise ValueError('Use a vMAJOR.MINOR.PATCH tag or dev-COMMITSHA')
    return value


def verify_binary(path, system, arch):
    if arch not in {'amd64', 'arm64'}:
        raise ValueError('Unsupported architecture')
    with Path(path).open('rb') as stream:
        header = stream.read(64)
        if system == 'windows':
            if header[:2] != b'MZ' or len(header) < 64:
                raise ValueError('Expected PE executable')
            stream.seek(struct.unpack_from('<I', header, 60)[0])
            pe = stream.read(6)
            expected = 0x8664 if arch == 'amd64' else 0xAA64
            valid = len(pe) == 6 and pe[:4] == b'PE\0\0' and struct.unpack_from('<H', pe, 4)[0] == expected
        elif system == 'linux':
            expected = 62 if arch == 'amd64' else 183
            valid = len(header) >= 20 and header[:6] == b'\x7fELF\x02\x01' and struct.unpack_from('<H', header, 18)[0] == expected
        elif system == 'macos':
            expected = 0x01000007 if arch == 'amd64' else 0x0100000C
            valid = len(header) >= 8 and header[:4] == b'\xcf\xfa\xed\xfe' and struct.unpack_from('<I', header, 4)[0] == expected
        else:
            raise ValueError('Unsupported operating system')
    if not valid:
        raise ValueError(f'Executable is not {system}/{arch}')


def digest(path):
    hasher = hashlib.sha256()
    with Path(path).open('rb') as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b''):
            hasher.update(block)
    return hasher.hexdigest()


def validate_assets(assets, system, arch):
    python = assets / 'python' / ('python.exe' if system == 'windows' else 'bin/python3')
    required = [python, assets / 'model/model.safetensors', assets / 'model/config.json',
                assets / 'model/tokenizer.json', assets / 'model/modules.json',
                assets / 'model-manifest.json', assets / 'THIRD-PARTY-NOTICES.txt']
    for path in required:
        if not path.is_file() or path.stat().st_size == 0:
            raise ValueError(f'Incomplete offline assets: {path}')
    manifest = json.loads((assets / 'model-manifest.json').read_text(encoding='utf-8'))
    if manifest.get('id') != 'google/embeddinggemma-2' or not manifest.get('offline'):
        raise ValueError('Expected an offline EmbeddingGemma 2 manifest')
    if manifest.get('revision') != '914f7f89142e33e77833254d9c9b90c3cef7303b':
        raise ValueError('Unexpected model revision')
    verify_binary(python, system, arch)


def smoke_resources(resources):
    result = subprocess.run([sys.executable, str(ROOT / 'scripts/smoke_model.py'), str(resources)],
                            check=True, text=True, capture_output=True, encoding='utf-8')
    report = json.loads(result.stdout)
    if not all(report.get(key) is True for key in ('offline', 'query', 'text', 'image')):
        raise ValueError('Offline inference smoke test failed')
    if report.get('socket_access') != 'denied' or report.get('dimensions') != 256:
        raise ValueError('Invalid offline inference report')
    return report


def sign_macos(app):
    subprocess.run(['xattr', '-cr', str(app)], check=True)
    magic = {b'\xfe\xed\xfa\xce', b'\xce\xfa\xed\xfe', b'\xfe\xed\xfa\xcf', b'\xcf\xfa\xed\xfe', b'\xca\xfe\xba\xbe', b'\xbe\xba\xfe\xca'}
    for path in (app / 'Contents/Resources/python').rglob('*'):
        if path.is_file():
            with path.open('rb') as stream:
                native = stream.read(4) in magic
            if native:
                subprocess.run(['codesign', '--force', '--sign', '-', str(path)], check=True,
                               stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    subprocess.run(['codesign', '--force', '--deep', '--sign', '-', str(app)], check=True)
    subprocess.run(['codesign', '--verify', '--deep', '--strict', str(app)], check=True)


def package(version, arch, output, assets=None):
    validate_version(version)
    system = SYSTEMS[sys.platform]
    if system == 'macos' and arch != 'arm64':
        raise ValueError('macOS Intel releases are not supported')
    assets = Path(assets) if assets is not None else ROOT / 'work/release-assets'
    validate_assets(assets, system, arch)
    output.mkdir(parents=True, exist_ok=True)
    suffix = '.tar.gz' if system == 'linux' else '.zip'
    archive = output / f'Shiwa-{version}-{system}-{arch}-full{suffix}'
    with tempfile.TemporaryDirectory(prefix='shiwa-release-') as temporary:
        stage = Path(temporary) / 'Shiwa'
        stage.mkdir()
        if system == 'macos':
            app = stage / '拾物.app'
            source = ROOT / 'build/bin/Shiwa.app'
            for relative in ('Contents/MacOS/Shiwa', 'Contents/Info.plist', 'Contents/Resources/AppIcon.icns'):
                destination = app / relative
                destination.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(source / relative, destination)
            verify_binary(app / 'Contents/MacOS/Shiwa', system, arch)
            info_path = app / 'Contents/Info.plist'
            info = plistlib.loads(info_path.read_bytes())
            if version.startswith('v'):
                info['CFBundleShortVersionString'] = re.match(r'v(\d+\.\d+\.\d+)', version)[1]
            info['CFBundleVersion'] = os.environ.get('GITHUB_RUN_NUMBER', '1')
            info['LSMinimumSystemVersion'] = '15.0'
            info_path.write_bytes(plistlib.dumps(info))
            resources = app / 'Contents/Resources'
        else:
            name = 'Shiwa.exe' if system == 'windows' else 'Shiwa'
            binary = ROOT / 'build/bin' / name
            verify_binary(binary, system, arch)
            shutil.copy2(binary, stage / name)
            resources = stage / 'Resources'
            resources.mkdir()
        shutil.copy2(ROOT / 'docs/CI-RELEASE.md', stage / 'README.md')
        for name in ('python', 'model'):
            shutil.copytree(assets / name, resources / name, symlinks=False)
        for name in ('model-manifest.json', 'THIRD-PARTY-NOTICES.txt'):
            shutil.copy2(assets / name, resources / name)
        if (assets / 'requirements-resolved.txt').is_file():
            shutil.copy2(assets / 'requirements-resolved.txt', resources)
        shutil.copy2(assets / 'THIRD-PARTY-NOTICES.txt', stage)
        if system == 'macos':
            sign_macos(app)
        # Run from a relocated directory with an empty cache and no sockets.
        report = smoke_resources(resources)
        report.update(system=system, arch=arch, version=version,
                      model_revision=json.loads((resources / 'model-manifest.json').read_text())['revision'])
        if system == 'linux':
            with tarfile.open(archive, 'w:gz') as bundle:
                bundle.add(stage, arcname='Shiwa')
        else:
            with zipfile.ZipFile(archive, 'w', zipfile.ZIP_DEFLATED) as bundle:
                for path in stage.rglob('*'):
                    if path.is_file():
                        bundle.write(path, path.relative_to(stage.parent))
            with zipfile.ZipFile(archive) as bundle:
                if bundle.testzip() is not None:
                    raise ValueError('Corrupt release archive')
    if archive.stat().st_size >= 2 * 1024**3:
        archive.unlink()
        raise ValueError('Archive exceeds the GitHub Release asset limit')
    archive.with_name(archive.name + '.smoke.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    archive.with_name(archive.name + '.sha256').write_text(f'{digest(archive)}  {archive.name}\n', encoding='utf-8')
    print(archive)
    return archive


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--version', required=True)
    parser.add_argument('--arch', required=True, choices=['amd64', 'arm64'])
    parser.add_argument('--assets', type=Path, default=ROOT / 'work/release-assets')
    parser.add_argument('--output', type=Path, default=ROOT / 'dist')
    args = parser.parse_args()
    package(args.version, args.arch, args.output.resolve(), args.assets.resolve())
