import argparse


def main() -> None:
    parser = argparse.ArgumentParser(description="Fine-tune a YOLO segmentation model for comic panels")
    parser.add_argument("--data", default="dataset.yaml")
    parser.add_argument("--base", default="yolo11n-seg.pt")
    parser.add_argument("--epochs", type=int, default=100)
    parser.add_argument("--image-size", type=int, default=1280)
    parser.add_argument("--batch", type=int, default=8)
    parser.add_argument("--device", default="")
    arguments = parser.parse_args()

    from ultralytics import YOLO

    model = YOLO(arguments.base)
    model.train(
        data=arguments.data,
        epochs=arguments.epochs,
        imgsz=arguments.image_size,
        batch=arguments.batch,
        device=arguments.device or None,
        project="runs/panel-segmentation",
        name="comic-panel-seg",
    )


if __name__ == "__main__":
    main()
