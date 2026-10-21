#!/usr/bin/env python3
"""Prepare pinned, redistributable Python/model assets on a native build host."""
import argparse
import json
import os
import platform
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[1]
MODEL = 'google/embeddinggemma-2'
REVISION = '914f7f89142e33e77833254d9c9b90c3cef7303b'
PYTHON_VERSION = '3.12.12'
UV_VERSION = '0.9.10'
VISION_REVISION = '1f1a920d685d46dfbe187934b785b7ecb7a182f2'


def python_path(resources, system=None):
    system = system or sys.platform
    return resources / 'python' / ('python.exe' if system == 'win32' else 'bin/python3')



def msvc_environment(env):
    installer = Path(os.environ.get('ProgramFiles(x86)', 'C:/Program Files (x86)')) / 'Microsoft Visual Studio/Installer/vswhere.exe'
    installation = subprocess.check_output([str(installer), '-latest', '-products', '*',
        '-requires', 'Microsoft.VisualStudio.Component.VC.Tools.ARM64', '-property', 'installationPath'], text=True).strip()
    if not installation:
        raise ValueError('Windows ARM64 release requires native MSVC ARM64 build tools')
    developer = Path(installation) / 'Common7/Tools/VsDevCmd.bat'
    # Pass cmd syntax directly: list2cmdline would backslash-escape the
    # embedded quotes, which cmd.exe does not interpret as quote escapes.
    command = f'cmd.exe /d /s /c "call "{developer}" -arch=arm64 -host_arch=arm64 >nul && set"'
    output = subprocess.check_output(command, executable=os.environ.get('COMSPEC', 'cmd.exe'), env=env, text=True)
    result = env.copy()
    for line in output.splitlines():
        key, separator, value = line.partition('=')
        if separator and key:
            for existing in list(result):
                if existing.lower() == key.lower():
                    del result[existing]
            result[key] = value
    return result


def build_windows_arm_vision(python, temporary, env, uv):
    # Upstream publishes torch for Windows ARM64 but no matching torchvision
    # wheel. Build the official pinned source on the native runner, not the user PC.
    subprocess.run([str(uv), 'pip', 'install', '--python', str(python), '--system', '--break-system-packages',
        '--only-binary', ':all:', '--torch-backend', 'cpu', 'torch==2.14.1', 'setuptools==84.0.0',
        'wheel==0.45.1', 'numpy==2.5.4', 'pillow==12.3.0'], env=env, check=True)
    source = Path(temporary) / 'torchvision-source'
    source.mkdir()
    subprocess.run(['git', 'init', str(source)], check=True)
    subprocess.run(['git', '-C', str(source), 'fetch', '--depth', '1',
        'https://github.com/pytorch/vision.git', VISION_REVISION], check=True)
    subprocess.run(['git', '-C', str(source), 'checkout', '--detach', 'FETCH_HEAD'], check=True)
    revision = subprocess.check_output(['git', '-C', str(source), 'rev-parse', 'HEAD'], text=True).strip()
    if revision != VISION_REVISION:
        raise ValueError('Unexpected torchvision source revision')
    build_env = msvc_environment(env)
    build_env.update(PYTHONHOME=str(python.parent), BUILD_VERSION='0.29.1', MAX_JOBS='2',
        FORCE_CUDA='0', TORCHVISION_USE_PNG='0', TORCHVISION_USE_JPEG='0',
        TORCHVISION_USE_WEBP='0', TORCHVISION_USE_NVJPEG='0')
    wheels = Path(temporary) / 'wheels'
    subprocess.run([str(python), 'setup.py', 'bdist_wheel', '--dist-dir', str(wheels)],
                   cwd=source, env=build_env, check=True)
    if not list(wheels.glob('torchvision-0.29.1-*-win_arm64.whl')):
        raise ValueError('Expected a native Windows ARM64 torchvision wheel')
    return wheels


def prepare(output):
    if output.exists():
        raise ValueError(f'Refusing to overwrite assets: {output}')
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='shiwa-assets-', dir=output.parent) as temporary:
        stage = Path(temporary) / 'Resources'
        stage.mkdir()
        env = os.environ.copy()
        env['UV_PYTHON_INSTALL_DIR'] = str(Path(temporary) / 'managed')
        env['UV_CACHE_DIR'] = str(Path(temporary) / 'uv-cache')
        tool = Path(temporary) / 'uv-tool'
        subprocess.run([sys.executable, '-m', 'pip', 'install', '--disable-pip-version-check',
                        '--target', str(tool), f'uv=={UV_VERSION}'], check=True)
        locate = f'import sys; sys.path.insert(0, {str(tool)!r}); import uv; print(uv.find_uv_bin())'
        uv = Path(subprocess.check_output([sys.executable, '-c', locate], text=True).strip())
        if not uv.is_file():
            raise ValueError('Missing private uv build tool')
        subprocess.run([str(uv), 'python', 'install', PYTHON_VERSION, '--no-bin', '--no-registry'], env=env, check=True)
        installed = list((Path(temporary) / 'managed').glob('cpython-*'))
        if len(installed) != 1:
            raise ValueError('Expected one managed CPython distribution')
        shutil.copytree(installed[0], stage / 'python', symlinks=False)
        python = python_path(stage)
        # Never resolve against a developer venv, user site or CUDA runtime.
        env.pop('PYTHONPATH', None)
        env.pop('PYTHONHOME', None)
        env['PYTHONNOUSERSITE'] = '1'
        # Managed CPython is marked EXTERNALLY-MANAGED. We only override that
        # marker for this private staged copy, never a host/system interpreter.
        wheel_options = []
        if sys.platform == 'win32' and platform.machine().lower() in {'arm64', 'aarch64'}:
            wheels = build_windows_arm_vision(python, temporary, env, uv)
            wheel_options = ['--find-links', str(wheels)]
        resolved = stage / 'requirements-resolved.txt'
        subprocess.run([str(uv), 'pip', 'compile', '--python', str(python),
                        '--only-binary', ':all:', '--torch-backend', 'cpu', '--generate-hashes',
                        str(ROOT / 'engine/requirements.txt'), '-c', str(ROOT / 'engine/requirements-lock.txt'),
                        '-o', str(resolved), *wheel_options], env=env, check=True)
        subprocess.run([str(uv), 'pip', 'install', '--python', str(python), '--system', '--break-system-packages',
                        '--only-binary', ':all:', '--torch-backend', 'cpu', '--require-hashes',
                        '-r', str(resolved), *wheel_options], env=env, check=True)
        code = ('from huggingface_hub import snapshot_download; '
                f'snapshot_download({MODEL!r}, revision={REVISION!r}, local_dir={str(stage / "model")!r}, '
                'ignore_patterns=["*.onnx", "*.tflite", "*.gguf", "*.h5", "*.msgpack"])')
        env['HF_HUB_DISABLE_XET'] = '1'
        env['HF_HUB_DISABLE_TELEMETRY'] = '1'
        env['PYTHONHOME'] = str(stage / 'python')
        subprocess.run([str(python), '-c', code], env=env, check=True)
        shutil.rmtree(stage / 'model/.cache', ignore_errors=True)
        (stage / 'model-manifest.json').write_text(json.dumps({
            'id': MODEL, 'revision': REVISION, 'offline': True,
            'python_version': PYTHON_VERSION, 'uv_version': UV_VERSION,
            'torchvision_source_revision': VISION_REVISION if wheel_options else None,
        }, indent=2) + '\n', encoding='utf-8')
        subprocess.run([sys.executable, str(ROOT / 'scripts/optimize_assets.py'), str(stage)], check=True)
        shutil.copy2(ROOT / 'engine/THIRD-PARTY-NOTICES.txt', stage)
        # Keep license metadata; remove generated caches, never copy user caches/settings.
        for cache in (stage / 'python').rglob('__pycache__'):
            shutil.rmtree(cache)
        shutil.move(str(stage), output)
    print(output)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, default=ROOT / 'work/release-assets')
    prepare(parser.parse_args().output.resolve())
