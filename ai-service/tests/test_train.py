import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import train


class TrainIntegrationTests(unittest.TestCase):
    def test_parser_preserves_training_defaults(self):
        arguments = train.build_parser().parse_args([])
        self.assertEqual(arguments.base, "yolo11n-seg.pt")
        self.assertEqual(arguments.epochs, 100)
        self.assertEqual(arguments.image_size, 1280)
        self.assertEqual(arguments.batch, 8)
        self.assertEqual(arguments.fraction, 1.0)

    def test_invalid_dataset_fails_before_ultralytics_import(self):
        with tempfile.TemporaryDirectory() as directory:
            missing = Path(directory) / "missing.yaml"
            with patch.dict("sys.modules", {"ultralytics": None}):
                result = train.main(["--data", str(missing)])
        self.assertEqual(result, 2)


if __name__ == "__main__":
    unittest.main()
