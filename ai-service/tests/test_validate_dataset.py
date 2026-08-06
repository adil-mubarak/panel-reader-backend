import tempfile
import unittest
from pathlib import Path

import yaml

from validate_dataset import validate_dataset


class DatasetValidationTests(unittest.TestCase):
    def make_dataset(self, label="0 0.1 0.1 0.9 0.1 0.9 0.9"):
        temporary = tempfile.TemporaryDirectory()
        root = Path(temporary.name)
        for split in ("train", "valid"):
            (root / split / "images").mkdir(parents=True)
            (root / split / "labels").mkdir(parents=True)
            (root / split / "images" / "page.jpg").touch()
            (root / split / "labels" / "page.txt").write_text(label, encoding="utf-8")
        data = root / "data.yaml"
        data.write_text(yaml.safe_dump({"path": ".", "train": "train/images", "val": "valid/images", "names": {0: "panel"}}), encoding="utf-8")
        return temporary, root, data

    def test_valid_segmentation_dataset(self):
        temporary, _, data = self.make_dataset()
        self.addCleanup(temporary.cleanup)
        summaries, errors = validate_dataset(data)
        self.assertEqual(errors, [])
        self.assertEqual(summaries["train"].polygons, 1)

    def test_absent_label_is_negative_image(self):
        temporary, root, data = self.make_dataset()
        self.addCleanup(temporary.cleanup)
        (root / "train" / "labels" / "page.txt").unlink()
        summaries, errors = validate_dataset(data)
        self.assertEqual(errors, [])
        self.assertEqual(summaries["train"].negatives, 1)

    def test_rejects_object_detection_label(self):
        temporary, _, data = self.make_dataset("0 0.5 0.5 0.4 0.4")
        self.addCleanup(temporary.cleanup)
        _, errors = validate_dataset(data)
        self.assertTrue(any("object-detection" in error for error in errors))

    def test_rejects_out_of_range_and_degenerate_polygons(self):
        temporary, root, data = self.make_dataset("0 0 0 1.1 0 1 1")
        self.addCleanup(temporary.cleanup)
        (root / "valid" / "labels" / "page.txt").write_text("0 0 0 0.5 0.5 1 1", encoding="utf-8")
        _, errors = validate_dataset(data)
        self.assertTrue(any("range 0..1" in error for error in errors))
        self.assertTrue(any("zero area" in error for error in errors))

    def test_missing_split_and_path(self):
        with tempfile.TemporaryDirectory() as directory:
            data = Path(directory) / "data.yaml"
            data.write_text("path: missing\nnames: [panel]\n", encoding="utf-8")
            _, errors = validate_dataset(data)
        self.assertTrue(any("dataset path does not exist" in error for error in errors))
        self.assertTrue(any("missing required split: train" in error for error in errors))
        self.assertTrue(any("missing required split: val" in error for error in errors))

    def test_rejects_extra_classes_and_orphan_labels(self):
        temporary, root, data = self.make_dataset()
        self.addCleanup(temporary.cleanup)
        data.write_text(yaml.safe_dump({"path": ".", "train": "train/images", "val": "valid/images", "names": {0: "panel", 1: "caption"}}), encoding="utf-8")
        (root / "train" / "labels" / "orphan.txt").write_text("0 0 0 1 0 1 1", encoding="utf-8")
        _, errors = validate_dataset(data)
        self.assertTrue(any("exactly one class" in error for error in errors))
        self.assertTrue(any("no matching image" in error for error in errors))


if __name__ == "__main__":
    unittest.main()
