import argparse
from pathlib import Path

from validate_dataset import validate_dataset


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Fine-tune a YOLO segmentation model for comic panels")
    parser.add_argument("--data", default="dataset.yaml")
    parser.add_argument("--base", default="yolo11n-seg.pt")
    parser.add_argument("--epochs", type=int, default=100)
    parser.add_argument("--image-size", type=int, default=1280)
    parser.add_argument("--batch", type=int, default=8)
    parser.add_argument("--device", default="")
    parser.add_argument("--workers", type=int, default=8)
    parser.add_argument("--patience", type=int, default=100)
    parser.add_argument("--seed", type=int, default=0)
    parser.add_argument("--fraction", type=float, default=1.0)
    parser.add_argument("--project", default=str(Path(__file__).resolve().parent / "runs" / "panel-segmentation"))
    parser.add_argument("--name", default="comic-panel-seg")
    return parser


def main(argv: list[str] | None = None) -> int:
    arguments = build_parser().parse_args(argv)

    _, errors = validate_dataset(arguments.data)
    if errors:
        print("Dataset validation failed. Run validate_dataset.py --data with the same path:")
        for error in errors:
            print(f"  - {error}")
        return 2

    from ultralytics import YOLO

    model = YOLO(arguments.base)
    result = model.train(
        data=arguments.data,
        epochs=arguments.epochs,
        imgsz=arguments.image_size,
        batch=arguments.batch,
        device=arguments.device or None,
        workers=arguments.workers,
        patience=arguments.patience,
        seed=arguments.seed,
        fraction=arguments.fraction,
        project=arguments.project,
        name=arguments.name,
    )
    save_dir = getattr(result, "save_dir", None) or getattr(getattr(model, "trainer", None), "save_dir", None)
    if save_dir:
        checkpoint = Path(save_dir) / "weights" / "best.pt"
        print(f"Training complete. Best checkpoint: {checkpoint}")
    else:
        print("Training complete. Check the Ultralytics run output above for the best.pt checkpoint path.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
