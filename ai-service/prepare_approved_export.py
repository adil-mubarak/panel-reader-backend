from __future__ import annotations

import argparse
import json
import random
import shutil
import zipfile
from pathlib import Path

import yaml


def prepare(source: Path, output: Path, seed: int = 42) -> dict[str, int]:
    source = source.resolve()
    output = output.resolve()
    if not source.is_file():
        raise ValueError(f"export ZIP not found: {source}")

    with zipfile.ZipFile(source) as archive:
        names = set(archive.namelist())
        images = sorted(name for name in names if name.startswith("images/") and not name.endswith("/"))
        if not images:
            raise ValueError("export contains no images")
        pairs: list[tuple[str, str]] = []
        for image in images:
            label = f"labels/{Path(image).stem}.txt"
            if label not in names:
                raise ValueError(f"missing label for {image}")
            pairs.append((image, label))

        rng = random.Random(seed)
        rng.shuffle(pairs)
        total = len(pairs)
        validation = max(1, round(total * 0.1))
        test = max(1, round(total * 0.1))
        train = total - validation - test
        if train < 1:
            raise ValueError("at least three approved pages are required")
        assignments = {
            "train": pairs[:train],
            "valid": pairs[train:train + validation],
            "test": pairs[train + validation:],
        }

        if output.exists():
            shutil.rmtree(output)
        for split, split_pairs in assignments.items():
            image_directory = output / split / "images"
            label_directory = output / split / "labels"
            image_directory.mkdir(parents=True, exist_ok=True)
            label_directory.mkdir(parents=True, exist_ok=True)
            for image, label in split_pairs:
                image_name = Path(image).name
                label_name = Path(label).name
                (image_directory / image_name).write_bytes(archive.read(image))
                (label_directory / label_name).write_bytes(archive.read(label))

        manifest = json.loads(archive.read("manifest.json")) if "manifest.json" in names else {}

    data = {
        "path": str(output),
        "train": "train/images",
        "val": "valid/images",
        "test": "test/images",
        "names": {0: "panel"},
    }
    (output / "data.yaml").write_text(yaml.safe_dump(data, sort_keys=False), encoding="utf-8")
    provenance = {
        "sourceExport": str(source),
        "seed": seed,
        "counts": {split: len(pairs) for split, pairs in assignments.items()},
        "exportManifest": manifest,
    }
    (output / "split-manifest.json").write_text(json.dumps(provenance, indent=2), encoding="utf-8")
    return provenance["counts"]


def main() -> int:
    parser = argparse.ArgumentParser(description="Split a Panel Reader YOLO export into a training dataset")
    parser.add_argument("--source", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--seed", type=int, default=42)
    arguments = parser.parse_args()
    try:
        counts = prepare(arguments.source, arguments.output, arguments.seed)
    except (OSError, ValueError, zipfile.BadZipFile, json.JSONDecodeError) as error:
        parser.error(str(error))
    print(" ".join(f"{split}={count}" for split, count in counts.items()))
    print(f"dataset={arguments.output.resolve() / 'data.yaml'}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
