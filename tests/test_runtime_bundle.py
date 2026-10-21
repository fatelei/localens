"""Build-host command guards without fetching or compiling inference libraries."""
import importlib.util
import os
import tempfile
from pathlib import Path
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location('runtime_bundle', Path(__file__).resolve().parents[1] / 'scripts/runtime_bundle.py')
runtime = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(runtime)


class RuntimeTests(unittest.TestCase):
    def test_msvc_command_preserves_space_path_quotes(self):
        with patch.object(runtime.subprocess, 'check_output', side_effect=['C:/Program Files/VS\n', 'Path=native-tools\nVSCMD_ARG_TGT_ARCH=arm64\n']) as command:
            result = runtime.msvc_environment({'PATH': 'old-tools', 'UNCHANGED': 'yes'})
        invocation = command.call_args_list[1].args[0]
        self.assertIsInstance(invocation, str)
        self.assertIn('call "C:/Program Files/VS', invocation.replace('\\', '/'))
        self.assertNotIn('\\"', invocation)
        self.assertIn('-arch=arm64 -host_arch=arm64', invocation)
        self.assertEqual(result['Path'], 'native-tools')
        self.assertNotIn('PATH', result)
        self.assertEqual(result['UNCHANGED'], 'yes')

    @unittest.skipUnless(os.name == 'nt', 'Exercises real Windows cmd quoting')
    def test_real_cmd_calls_batch_inside_program_files_path(self):
        with tempfile.TemporaryDirectory() as temporary:
            installation = Path(temporary) / 'Program Files' / 'VS'
            developer = installation / 'Common7/Tools/VsDevCmd.bat'
            developer.parent.mkdir(parents=True)
            developer.write_text('@echo off\nset SHIWA_COMPILER_QUOTE_TEST=ok\n', encoding='ascii')
            original = runtime.subprocess.check_output
            calls = 0

            def execute(*args, **kwargs):
                nonlocal calls
                calls += 1
                if calls == 1:
                    return str(installation)
                return original(*args, **kwargs)

            with patch.object(runtime.subprocess, 'check_output', side_effect=execute):
                result = runtime.msvc_environment(os.environ.copy())
            self.assertEqual(result['SHIWA_COMPILER_QUOTE_TEST'], 'ok')

    def test_missing_native_compiler_is_an_error(self):
        with patch.object(runtime.subprocess, 'check_output', return_value=''):
            with self.assertRaisesRegex(ValueError, 'MSVC ARM64'):
                runtime.msvc_environment({})

    def test_native_vision_bootstrap_targets_only_private_python(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            wheels = root / 'wheels'
            wheels.mkdir()
            (wheels / 'torchvision-0.29.1-cp312-cp312-win_arm64.whl').write_bytes(b'fixture')
            python = root / 'Resources/python/python.exe'
            with patch.object(runtime.subprocess, 'run') as run, patch.object(runtime.subprocess, 'check_output', return_value=runtime.VISION_REVISION), patch.object(runtime, 'msvc_environment', return_value={}):
                result = runtime.build_windows_arm_vision(python, root, {}, root / 'uv.exe')
            self.assertEqual(result, wheels)
            install = run.call_args_list[0].args[0]
            self.assertIn('--break-system-packages', install)
            self.assertEqual(install[install.index('--python') + 1], str(python))
            build = run.call_args_list[-1]
            self.assertEqual(build.args[0][0], str(python))
            self.assertEqual(build.kwargs['env']['PYTHONHOME'], str(python.parent))

    def test_runtime_layout_is_platform_specific(self):
        root = Path('Resources')
        self.assertEqual(runtime.python_path(root, 'win32'), root / 'python/python.exe')
        self.assertEqual(runtime.python_path(root, 'linux'), root / 'python/bin/python3')


if __name__ == '__main__':
    unittest.main()
