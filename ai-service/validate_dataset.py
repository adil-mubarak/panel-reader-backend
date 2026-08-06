from __future__ import annotations

import argparse
import math
import sys
from dataclasses import dataclass
from pathlib import Path

try:
    import yaml
except ImportError as error:  # pragma: no cover - depends on the environment
    raise SystemExit(
        "PyYAML is required to read YOLO dataset files. Install requirements-local.txt."
    ) from error


IMAGE_EXTENSIONS = {".bmp", ".dng", ".jpeg", ".jpg", ".mpo", ".png", ".tif", ".tiff", ".webp"}


@dataclass
class SplitSummary:
    images: int = 0
    negatives: int = 0
    polygons: int = 0


def _label_directory(image_directory: Path) -> Path:
    parts = list(image_directory.parts)
    indices = [index for index, part in enumerate(parts) if part == "images"]
    if not indices:
        raise ValueError(f"image directory must contain an 'images' path component: {image_directory}")
    parts[indices[-1]] = "labels"
    return Path(*parts)


def _names(value: object) -> dict[int, str]:
    if isinstance(value, list):
        return {index: str(name) for index, name in enumerate(value)}
    if isinstance(value, dict):
        try:
            return {int(index): str(name) for index, name in value.items()}
        except (TypeError, ValueError) as error:
            raise ValueError("names keys must be integer class IDs") from error
    raise ValueError("names must be a list or mapping")


def _polygon_error(tokens: list[str], names: dict[int, str]) -> str | None:
    if len(tokens) == 5:
        return "object-detection label has 5 tokens; export YOLO segmentation labels"
    if len(tokens) < 7 or len(tokens) % 2 == 0:
        return "segmentation label requires a class and at least 3 XY point pairs"
    try:
        class_id = int(tokens[0])
    except ValueError:
        return "class ID must be an integer"
    if class_id not in names:
        return f"unknown class ID {class_id}"
    try:
        coordinates = [float(value) for value in tokens[1:]]
    except ValueError:
        return "coordinates must be numbers"
    if any(not math.isfinite(value) for value in coordinates):
        return "coordinates must be finite"
    if any(value < 0 or value > 1 for value in coordinates):
        return "coordinates must be normalized to the range 0..1"
    points = list(zip(coordinates[::2], coordinates[1::2]))
    if len(set(points)) != len(points):
        return "polygon contains duplicate points"
    area = abs(sum(
        x1 * y2 - x2 * y1
        for (x1, y1), (x2, y2) in zip(points, points[1:] + points[:1])
    )) / 2
    if area <= 1e-12:
        return "polygon has zero area (degenerate or collinear points)"
    return None


def validate_dataset(data_path: str | Path) -> tuple[dict[str, SplitSummary], list[str]]:
    source = Path(data_path).expanduser().resolve()
    errors: list[str] = []
    summaries: dict[str, SplitSummary] = {}
    if not source.is_file():
        return summaries, [f"dataset YAML not found: {source}"]
    try:
        document = yaml.safe_load(source.read_text(encoding="utf-8"))
    except (OSError, yaml.YAMLError) as error:
        return summaries, [f"cannot read dataset YAML: {error}"]
    if not isinstance(document, dict):
        return summaries, ["dataset YAML must contain a mapping"]
    try:
        names = _names(document.get("names"))
    except ValueError as error:
        names = {}
        errors.append(str(error))
    if names.get(0) != "panel":
        errors.append("class 0 must be named 'panel'")
    if names and names != {0: "panel"}:
        errors.append("dataset must contain exactly one class: 0: panel")

    configured_root = document.get("path", ".")
    if not isinstance(configured_root, str) or not configured_root.strip():
        errors.append("path must be a non-empty string")
        root = source.parent
    else:
        root = Path(configured_root).expanduser()
        if not root.is_absolute():
            root = source.parent / root
        root = root.resolve()
    if not root.is_dir():
        errors.append(f"dataset path does not exist: {root}")

    for split in ("train", "val", "test"):
        configured = document.get(split)
        if configured is None:
            if split in {"train", "val"}:
                errors.append(f"missing required split: {split}")
            continue
        if not isinstance(configured, str) or not configured.strip():
            errors.append(f"{split} must be a non-empty image directory path")
            continue
        image_directory = Path(configured)
        if not image_directory.is_absolute():
            image_directory = root / image_directory
        image_directory = image_directory.resolve()
        summary = summaries.setdefault(split, SplitSummary())
        if not image_directory.is_dir():
            errors.append(f"{split} image directory does not exist: {image_directory}")
            continue
        try:
            label_directory = _label_directory(image_directory)
        except ValueError as error:
            errors.append(f"{split}: {error}")
            continue
        images = sorted(path for path in image_directory.rglob("*") if path.is_file() and path.suffix.lower() in IMAGE_EXTENSIONS)
        summary.images = len(images)
        if not images:
            errors.append(f"{split} contains no supported images: {image_directory}")
        for image in images:
            relative = image.relative_to(image_directory).with_suffix(".txt")
            label = label_directory / relative
            if not label.exists():
                summary.negatives += 1
                continue
            if not label.is_file():
                errors.append(f"{split}: label is not a file: {label}")
                continue
            try:
                contents = label.read_text(encoding="utf-8")
            except (OSError, UnicodeError) as error:
                errors.append(f"{split}: cannot read {label}: {error}")
                continue
            if not contents.strip():
                summary.negatives += 1
                continue
            lines = contents.splitlines()
            for line_number, line in enumerate(lines, 1):
                if not line.strip():
                    continue
                problem = _polygon_error(line.split(), names)
                if problem:
                    errors.append(f"{split}: {label}:{line_number}: {problem}")
                else:
                    summary.polygons += 1
        if label_directory.is_dir():
            image_stems = {image.relative_to(image_directory).with_suffix("") for image in images}
            for label in label_directory.rglob("*.txt"):
                relative = label.relative_to(label_directory).with_suffix("")
                if relative not in image_stems:
                    errors.append(f"{split}: label has no matching image: {label}")
    return summaries, errors


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Validate a YOLO instance-segmentation dataset")
    parser.add_argument("--data", required=True, help="path to the YOLO dataset YAML")
    arguments = parser.parse_args(argv)
    summaries, errors = validate_dataset(arguments.data)
    for split in ("train", "val", "test"):
        if split in summaries:
            value = summaries[split]
            print(f"{split}: images={value.images} negative_images={value.negatives} polygons={value.polygons}")
    print(f"errors={len(errors)}")
    for error in errors:
        print(f"ERROR: {error}", file=sys.stderr)
    return 1 if errors else 0


if __name__ == "__main__":
    raise SystemExit(main())
