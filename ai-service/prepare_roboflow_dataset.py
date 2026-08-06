from __future__ import annotations

import argparse
import os
import shutil
from pathlib import Path

import yaml


IMAGE_EXTENSIONS = {".bmp", ".jpeg", ".jpg", ".png", ".tif", ".tiff", ".webp"}


def prepare(source: Path, output: Path, panel_class: str) -> dict[str, int]:
    source = source.resolve()
    output = output.resolve()
    data_path = source / "data.yaml"
    if not data_path.is_file():
        raise ValueError(f"Roboflow data.yaml not found: {data_path}")
    document = yaml.safe_load(data_path.read_text(encoding="utf-8"))
    names = document.get("names", [])
    if isinstance(names, dict):
        classes = {int(index): str(name) for index, name in names.items()}
    else:
        classes = {index: str(name) for index, name in enumerate(names)}
    matches = [index for index, name in classes.items() if name.casefold() == panel_class.casefold()]
    if len(matches) != 1:
        raise ValueError(f"expected exactly one class named {panel_class!r}; found {classes}")
    panel_id = matches[0]

    counts = {"images": 0, "panels": 0, "dropped_annotations": 0, "cleaned_polygons": 0, "negative_images": 0}
    available_splits: list[str] = []
    for split in ("train", "valid", "test"):
        source_images = source / split / "images"
        source_labels = source / split / "labels"
        if not source_images.is_dir():
            if split == "test":
                continue
            raise ValueError(f"missing Roboflow split: {source_images}")
        available_splits.append(split)
        target_images = output / split / "images"
        target_labels = output / split / "labels"
        target_images.mkdir(parents=True, exist_ok=True)
        target_labels.mkdir(parents=True, exist_ok=True)
        for image in sorted(path for path in source_images.rglob("*") if path.is_file() and path.suffix.lower() in IMAGE_EXTENSIONS):
            relative = image.relative_to(source_images)
            target_image = target_images / relative
            target_image.parent.mkdir(parents=True, exist_ok=True)
            if target_image.exists():
                target_image.unlink()
            try:
                os.link(image, target_image)
            except OSError:
                shutil.copy2(image, target_image)
            counts["images"] += 1

            source_label = source_labels / relative.with_suffix(".txt")
            target_label = target_labels / relative.with_suffix(".txt")
            target_label.parent.mkdir(parents=True, exist_ok=True)
            retained: list[str] = []
            if source_label.is_file():
                for line in source_label.read_text(encoding="utf-8").splitlines():
                    tokens = line.split()
                    if not tokens:
                        continue
                    if int(tokens[0]) == panel_id:
                        coordinates = tokens[1:]
                        points = list(zip(coordinates[::2], coordinates[1::2]))
                        cleaned: list[tuple[str, str]] = []
                        seen: set[tuple[str, str]] = set()
                        for point in points:
                            if point not in seen:
                                cleaned.append(point)
                                seen.add(point)
                        if len(cleaned) < 3:
                            counts["dropped_annotations"] += 1
                            continue
                        if len(cleaned) != len(points):
                            counts["cleaned_polygons"] += 1
                        retained.append("0 " + " ".join(value for point in cleaned for value in point))
                        counts["panels"] += 1
                    else:
                        counts["dropped_annotations"] += 1
            target_label.write_text("\n".join(retained) + ("\n" if retained else ""), encoding="utf-8")
            if not retained:
                counts["negative_images"] += 1

    output.mkdir(parents=True, exist_ok=True)
    normalized: dict[str, object] = {
        "path": str(output),
        "train": "train/images",
        "val": "valid/images",
        "names": {0: "panel"},
    }
    if "test" in available_splits:
        normalized["test"] = "test/images"
    (output / "data.yaml").write_text(yaml.safe_dump(normalized, sort_keys=False), encoding="utf-8")
    roboflow = document.get("roboflow", {})
    attribution = (
        "Comic Book Panel Detection 2 Dataset\n"
        f"Source: {roboflow.get('url', 'https://universe.roboflow.com/test-qu9k2/comic-book-panel-detection-2')}\n"
        f"License: {roboflow.get('license', 'CC BY 4.0')}\n"
        "Changes: Cover annotations removed; Panels remapped to the single class panel.\n"
    )
    (output / "ATTRIBUTION.txt").write_text(attribution, encoding="utf-8")
    return counts


def main() -> int:
    parser = argparse.ArgumentParser(description="Normalize a Roboflow YOLO segmentation export for panel training")
    parser.add_argument("--source", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--panel-class", default="Panels")
    arguments = parser.parse_args()
    try:
        counts = prepare(arguments.source, arguments.output, arguments.panel_class)
    except (OSError, ValueError, yaml.YAMLError) as error:
        parser.error(str(error))
    print(" ".join(f"{key}={value}" for key, value in counts.items()))
    print(f"dataset={arguments.output.resolve() / 'data.yaml'}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
