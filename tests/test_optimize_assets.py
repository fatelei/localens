"""Keep inference tensors/licenses while stripping build-only files on both layouts."""
import json
from pathlib import Path
import struct
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]


class OptimizeTests(unittest.TestCase):
    def test_windows_and_unix_assets_keep_text_vision_and_licenses(self):
        with tempfile.TemporaryDirectory() as temporary:
            resources = Path(temporary)
            model = resources / 'model'
            model.mkdir()
            tensors = {
                'vision_tower.weight': {'dtype': 'F32', 'shape': [1], 'data_offsets': [0, 4]},
                'audio_tower.weight': {'dtype': 'F32', 'shape': [1], 'data_offsets': [4, 8]},
            }
            encoded = json.dumps(tensors).encode()
            (model / 'model.safetensors').write_bytes(struct.pack('<Q', len(encoded)) + encoded + b'KEEPDROP')
            (resources / 'model-manifest.json').write_text('{}')
            for relative in ('python/Lib/site-packages', 'python/lib/python3.12/site-packages'):
                site = resources / relative
                for name in ('torch/include', 'pip', 'setuptools', 'wheel'):
                    (site / name).mkdir(parents=True)
                    (site / name / 'build-only').write_text('unused')
                (site / 'torch-2.14.1.dist-info').mkdir()
                (site / 'torch-2.14.1.dist-info/LICENSE').write_text('preserve')
            subprocess.run([sys.executable, str(ROOT / 'scripts/optimize_assets.py'), str(resources)], check=True, capture_output=True)
            weight = (model / 'model.safetensors').read_bytes()
            length = struct.unpack('<Q', weight[:8])[0]
            self.assertEqual(weight[8 + length:], b'KEEP')
            self.assertEqual(set(json.loads(weight[8:8 + length])), {'vision_tower.weight'})
            for relative in ('python/Lib/site-packages', 'python/lib/python3.12/site-packages'):
                site = resources / relative
                self.assertFalse((site / 'torch/include').exists())
                self.assertFalse((site / 'pip').exists())
                self.assertTrue((site / 'torch-2.14.1.dist-info/LICENSE').is_file())


if __name__ == '__main__':
    unittest.main()
