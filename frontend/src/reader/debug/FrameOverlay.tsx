import { FocusFrame, frameBounds } from "../engine/camera";

export function FrameOverlay({ frames, selected, onSelect }: { frames: FocusFrame[]; selected?: number; onSelect?: (index: number) => void }) {
  return (
    <div className="frame-overlay">
      {frames.map((frame, index) => (
        (() => {
          const bounds = frameBounds(frame);
          const polygon = frame.shapeType === "polygon"
            ? frame.polygon.map((point) => `${((point.x - bounds.x) / bounds.width) * 100}% ${((point.y - bounds.y) / bounds.height) * 100}%`).join(",")
            : undefined;
          return (
        <button
          className={`frame-box${selected === index ? " selected" : ""}`}
          key={frame.id ?? `${frame.order}-${index}`}
          onClick={() => onSelect?.(index)}
          style={{ left: `${bounds.x * 100}%`, top: `${bounds.y * 100}%`, width: `${bounds.width * 100}%`, height: `${bounds.height * 100}%`, clipPath: polygon ? `polygon(${polygon})` : undefined }}
          type="button"
        >
          <span>Frame {index + 1}</span>
        </button>
          );
        })()
      ))}
    </div>
  );
}
