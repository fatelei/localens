#!/usr/bin/env python3
"""Exercise relocated bundled Python and text/image inference with sockets denied."""
import argparse
import json
import math
import os
from pathlib import Path
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[1]


def smoke(resources):
    resources = resources.resolve()
    python = resources / 'python' / ('python.exe' if sys.platform == 'win32' else 'bin/python3')
    env = os.environ.copy()
    for key in ('PYTHONPATH', 'PYTHONHOME', 'HF_TOKEN', 'HUGGING_FACE_HUB_TOKEN', 'LOCALENS_MODEL_PATH'):
        env.pop(key, None)
    env.update(PYTHONHOME=str(resources / 'python'), PYTHONNOUSERSITE='1', PYTHONDONTWRITEBYTECODE='1', PYTHONIOENCODING='utf-8', PYTHONUTF8='1',
               LOCALENS_MODEL_PATH=str(resources / 'model'), HF_HUB_OFFLINE='1', TRANSFORMERS_OFFLINE='1',
               HF_HUB_DISABLE_TELEMETRY='1', HF_HUB_DISABLE_XET='1', OMP_NUM_THREADS='2')
    with tempfile.TemporaryDirectory(prefix='shiwa-offline-') as temporary:
        temporary = Path(temporary)
        env['HF_HOME'] = str(temporary / 'empty-hf-cache')
        script = temporary / 'offline_worker.py'
        # Exercise the same embedded worker as the application. Deny outbound
        # socket calls even if a library accidentally ignores the offline flags.
        script.write_text('import socket, runpy\n'
                          'def denied(*args, **kwargs): raise RuntimeError("Network forbidden in release smoke test")\n'
                          'socket.socket.connect = denied\nsocket.socket.connect_ex = denied\n'
                          'socket.create_connection = denied\nsocket.getaddrinfo = denied\n'
                          f'runpy.run_path({str(ROOT / "engine/worker.py")!r}, run_name="__main__")\n', encoding='utf-8')
        # A tiny PPM fixture works through Pillow without external test assets.
        image = temporary / 'fixture.ppm'
        image.write_bytes(b'P6\n8 8\n255\n' + bytes([255, 120, 20]) * 64)
        requests = [
            {'kind': 'query', 'text': 'sunset over the sea'},
            {'kind': 'text', 'text': 'A photo of a sunset over the sea.'},
            {'kind': 'image', 'path': str(image)},
        ]
        completed = subprocess.run([str(python), '-u', str(script)],
                                   input=''.join(json.dumps(r) + '\n' for r in requests),
                                   text=True, capture_output=True, encoding='utf-8', env=env,
                                   cwd=temporary, timeout=900, check=True)
        replies = [json.loads(line) for line in completed.stdout.splitlines() if line.strip()]
        if len(replies) != 4 or replies[0] != {'ready': True}:
            raise ValueError(f'Bundled model did not become ready: {replies!r}\n{completed.stderr[-2000:]}')
        for reply in replies[1:]:
            vector = reply.get('vector', [])
            if len(vector) != 256 or not all(isinstance(v, (float, int)) and math.isfinite(v) for v in vector):
                raise ValueError(f'Invalid bundled model vector: {reply!r}')
            if not math.isclose(sum(v*v for v in vector), 1.0, abs_tol=0.01):
                raise ValueError('Bundled model vector is not normalized')
    return {'offline': True, 'socket_access': 'denied', 'dimensions': 256, 'query': True, 'text': True, 'image': True}


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('resources', type=Path)
    print(json.dumps(smoke(parser.parse_args().resources), indent=2))
