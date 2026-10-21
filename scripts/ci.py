#!/usr/bin/env python3
"""Dependency-free CI entry point; run from any working directory."""
import argparse
import ast
import os
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
SKIP = {'.git', '.venv', '.models', '__pycache__', 'bin', 'dist', 'work'}


def run(*args):
    subprocess.run(args, cwd=ROOT, check=True)


def sources():
    for directory, dirs, files in os.walk(ROOT):
        dirs[:] = [name for name in dirs if name not in SKIP]
        for name in files:
            yield Path(directory) / name


def lint():
    files = list(sources())
    go_files = [str(p.relative_to(ROOT)) for p in files if p.suffix == '.go']
    result = subprocess.check_output(['gofmt', '-l', *go_files], cwd=ROOT, text=True)
    if result.strip():
        raise SystemExit('Run gofmt on:\n' + result)
    for path in files:
        if path.suffix == '.py':
            ast.parse(path.read_text(encoding='utf-8'), filename=str(path))
        elif path.suffix in {'.js', '.cjs'}:
            run('node', '--check', str(path))


def test():
    system = sys.platform
    tags = 'sqlite_fts5,webkit2_41' if system.startswith('linux') else 'sqlite_fts5'
    run('go', 'vet', '-tags', tags, './...')
    args = ['go', 'test', '-timeout', '8m', '-tags', tags, './...']
    if system != 'win32':
        args.insert(2, '-race')
    if system.startswith('linux'):
        args = ['xvfb-run', '-a', *args]
    run(*args)
    run('node', '--test', 'tests/frontend.test.cjs', 'tests/thumbnails.test.cjs')
    run(sys.executable, '-m', 'unittest', 'discover', '-s', 'tests', '-p', 'test_*.py')


def build():
    host = subprocess.check_output(['go', 'env', 'GOHOSTARCH'], text=True).strip()
    expected = os.environ.get('CI_TARGET_ARCH', host)
    if host != expected:
        raise SystemExit(f'Expected native {expected} runner, got {host}')
    if sys.platform == 'win32':
        (ROOT / 'build/bin').mkdir(parents=True, exist_ok=True)
        run('go', 'build', '-trimpath', '-tags', 'desktop,production,sqlite_fts5',
            '-ldflags', '-s -w -H windowsgui', '-o', 'build/bin/Shiwa.exe', '.')
    else:
        run('sh', 'scripts/build.sh')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('command', choices=['lint', 'test', 'build'])
    globals()[parser.parse_args().command]()
