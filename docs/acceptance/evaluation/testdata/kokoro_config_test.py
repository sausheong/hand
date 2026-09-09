"""Offline oracle for config validation; never loads a model or downloads data."""
import importlib.util
from pathlib import Path
import sys
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('acceptance_config', Path('src/kokoro_onnx/config.py'))
config = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = config
spec.loader.exec_module(config)


class ConfigPaths(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.model = self.root/'model.onnx'
        self.voices = self.root/'voices.bin'
        self.model.write_bytes(b'model placeholder: validation must not load it')
        self.voices.write_bytes(b'voices placeholder: validation must not load it')

    def validate(self):
        return config.KoKoroConfig(str(self.model), str(self.voices)).validate()

    def test_voices_directory_is_rejected(self):
        self.voices.unlink()
        self.voices.mkdir()
        with self.assertRaises(IsADirectoryError) as failure:
            self.validate()
        self.assertIn(str(self.voices), str(failure.exception))

    def test_model_directory_is_rejected(self):
        self.model.unlink()
        self.model.mkdir()
        with self.assertRaises(IsADirectoryError) as failure:
            self.validate()
        self.assertIn(str(self.model), str(failure.exception))

    def test_missing_voices_retains_download_hint(self):
        self.voices.unlink()
        with self.assertRaises(FileNotFoundError) as failure:
            self.validate()
        self.assertIn(str(self.voices), str(failure.exception))
        self.assertIn('download', str(failure.exception).lower())

    def test_missing_model_retains_download_hint(self):
        self.model.unlink()
        with self.assertRaises(FileNotFoundError) as failure:
            self.validate()
        self.assertIn(str(self.model), str(failure.exception))
        self.assertIn('download', str(failure.exception).lower())

    def test_regular_files_and_symlinks_preserve_configuration(self):
        espeak = config.EspeakConfig(lib_path='library', data_path='data')
        original = config.KoKoroConfig(str(self.model), str(self.voices), espeak)
        self.assertIsNone(original.validate())
        self.assertEqual(original.model_path, str(self.model))
        self.assertEqual(original.voices_path, str(self.voices))
        self.assertIs(original.espeak_config, espeak)
        model_link, voices_link = self.root/'model-link', self.root/'voices-link'
        model_link.symlink_to(self.model)
        voices_link.symlink_to(self.voices)
        self.assertIsNone(config.KoKoroConfig(str(model_link), str(voices_link)).validate())
