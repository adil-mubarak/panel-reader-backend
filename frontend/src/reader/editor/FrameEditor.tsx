import { PointerEvent, useEffect, useRef, useState } from "react";
import { FocusFrame, defaultFrame, frameBounds } from "../engine/camera";
import { FrameOverlay } from "../debug/FrameOverlay";

interface DragState {
  index: number;
  mode: "move" | "polygon-move" | "vertex" | "n" | "ne" | "e" | "se" | "s" | "sw" | "w" | "nw";
  pointIndex?: number;
  startX: number;
  startY: number;
  frame: FocusFrame;
}

interface DetectionReport {
  warnings: string[];
  panelCount: number;
  coverage: number;
  averageConfidence: number;
}

export function FrameEditor({ image, initialFrames, reviewStatus, detectionReport, onSave, onDetect, onApprove, onExport, onCancel }: {
  image: string;
  initialFrames: FocusFrame[];
  reviewStatus: string;
  detectionReport: DetectionReport;
  onSave: (frames: FocusFrame[]) => Promise<void>;
  onDetect: (reset: boolean) => Promise<FocusFrame[]>;
  onApprove: (approved: boolean) => Promise<void>;
  onExport: (format: "yolo" | "coco") => void;
  onCancel: () => void;
}) {
  const [frames, setFrames] = useState(() => initialFrames.map((frame) => ({ ...frame })));
  const [selected, setSelected] = useState(0);
  const [selectedFrames, setSelectedFrames] = useState<Set<number>>(() => new Set([0]));
  const [drag, setDrag] = useState<DragState | null>(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [detecting, setDetecting] = useState(false);
  const [approving, setApproving] = useState(false);
  const [draggedOrder, setDraggedOrder] = useState<number | null>(null);
  const [dirty, setDirty] = useState(false);
  const [undo, setUndo] = useState<FocusFrame[][]>([]);
  const [redo, setRedo] = useState<FocusFrame[][]>([]);
  const framesRef = useRef(frames);
  framesRef.current = frames;

  useEffect(() => {
    if (!dirty || drag) return;
    const timer = window.setTimeout(() => void save(), 900);
    return () => window.clearTimeout(timer);
  }, [dirty, drag, frames]);

  useEffect(() => {
    function warn(event: BeforeUnloadEvent) {
      if (!dirty) return;
      event.preventDefault();
    }
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty]);

  function commit(update: (current: FocusFrame[]) => FocusFrame[]) {
    setFrames((current) => {
      setUndo((history) => [...history.slice(-29), current.map((frame) => ({ ...frame, polygon: [...frame.polygon] }))]);
      setRedo([]);
      return update(current);
    });
    setDirty(true);
  }

  function startDrag(event: PointerEvent, mode: DragState["mode"], pointIndex?: number) {
    if (!frames[selected]) return;
    const bounds = event.currentTarget.closest(".editor-canvas")!.getBoundingClientRect();
    setUndo((history) => [...history.slice(-29), frames.map((frame) => ({ ...frame, polygon: frame.polygon.map((point) => ({ ...point })) }))]);
    setRedo([]);
    setDrag({ index: selected, mode, pointIndex, startX: (event.clientX - bounds.left) / bounds.width, startY: (event.clientY - bounds.top) / bounds.height, frame: { ...frames[selected], polygon: frames[selected].polygon.map((point) => ({ ...point })) } });
    event.currentTarget.setPointerCapture(event.pointerId);
    event.stopPropagation();
  }

  function moveDrag(event: PointerEvent<HTMLDivElement>) {
    if (!drag) return;
    const bounds = event.currentTarget.getBoundingClientRect();
    const x = (event.clientX - bounds.left) / bounds.width;
    const y = (event.clientY - bounds.top) / bounds.height;
    const dx = x - drag.startX;
    const dy = y - drag.startY;
    setFrames((current) => current.map((frame, index) => {
      if (index !== drag.index) return frame;
      if (drag.mode === "vertex" && drag.pointIndex !== undefined) {
        return {
          ...frame,
          polygon: drag.frame.polygon.map((point, pointIndex) => pointIndex === drag.pointIndex ? { x: clamp(x, 0, 1), y: clamp(y, 0, 1) } : point),
          source: frame.source === "detected" ? "manual_edited" : "manual",
        };
      }
      if (drag.mode === "polygon-move") {
        const bounds = frameBounds(drag.frame);
        const adjustedX = clamp(dx, -bounds.x, 1 - bounds.x - bounds.width);
        const adjustedY = clamp(dy, -bounds.y, 1 - bounds.y - bounds.height);
        return {
          ...frame,
          polygon: drag.frame.polygon.map((point) => ({ x: point.x + adjustedX, y: point.y + adjustedY })),
          source: frame.source === "detected" ? "manual_edited" : "manual",
        };
      }
      if (drag.mode === "move") {
        return { ...frame, x: clamp(drag.frame.x + dx, 0, 1 - frame.width), y: clamp(drag.frame.y + dy, 0, 1 - frame.height), source: frame.source === "detected" ? "manual_edited" : "manual" };
      }
      let left = drag.frame.x;
      let right = drag.frame.x + drag.frame.width;
      let top = drag.frame.y;
      let bottom = drag.frame.y + drag.frame.height;
      if (drag.mode.includes("w")) left = clamp(left + dx, 0, right - 0.03);
      if (drag.mode.includes("e")) right = clamp(right + dx, left + 0.03, 1);
      if (drag.mode.includes("n")) top = clamp(top + dy, 0, bottom - 0.03);
      if (drag.mode.includes("s")) bottom = clamp(bottom + dy, top + 0.03, 1);
      return { ...frame, x: left, y: top, width: right - left, height: bottom - top, source: frame.source === "detected" ? "manual_edited" : "manual" };
    }));
    setDirty(true);
  }

  function addFrame() {
    const offset = (frames.length % 5) * 0.035;
    commit((current) => [...current, defaultFrame(current.length + 1, { x: 0.1 + offset, y: 0.1 + offset })]);
    setSelected(frames.length);
    setSelectedFrames(new Set([frames.length]));
  }

  function addPolygon() {
    commit((current) => [...current, defaultFrame(current.length + 1, {
      name: `Polygon ${current.length + 1}`,
      shapeType: "polygon",
      frameType: "focus",
      x: 0,
      y: 0,
      width: 0,
      height: 0,
      polygon: [{ x: 0.15, y: 0.15 }, { x: 0.85, y: 0.15 }, { x: 0.75, y: 0.45 }, { x: 0.2, y: 0.45 }],
    })]);
    setSelected(frames.length);
    setSelectedFrames(new Set([frames.length]));
  }

  function addFullPage() {
    commit((current) => [...current, defaultFrame(current.length + 1, { name: "Full page", frameType: "full_page", x: 0, y: 0, width: 1, height: 1, paddingPercent: 2, maskOpacity: 0 })]);
    setSelected(frames.length);
    setSelectedFrames(new Set([frames.length]));
  }

  function duplicateFrame() {
    if (!frames[selected]) return;
    commit((current) => [...current, { ...current[selected], id: undefined, name: `${current[selected].name} copy`, order: current.length + 1, source: "manual", polygon: current[selected].polygon.map((point) => ({ ...point })) }]);
    setSelected(frames.length);
    setSelectedFrames(new Set([frames.length]));
  }

  function removeFrame() {
    if (frames.length <= 1) return;
    const removing = selectedFrames.size ? selectedFrames : new Set([selected]);
    if (frames.length - removing.size < 1) return;
    commit((current) => current.filter((_, index) => !removing.has(index)).map((frame, index) => ({ ...frame, order: index + 1 })));
    const next = Math.max(0, Math.min(selected, frames.length - removing.size - 1));
    setSelected(next);
    setSelectedFrames(new Set([next]));
  }

  function splitFrame(axis: "horizontal" | "vertical") {
    const frame = frames[selected];
    if (!frame || frame.shapeType !== "rectangle") return;
    const first = { ...frame, id: undefined, name: `${frame.name} A`, source: "manual" as const, confidence: undefined, modelVersion: undefined };
    const second = { ...frame, id: undefined, name: `${frame.name} B`, source: "manual" as const, confidence: undefined, modelVersion: undefined };
    if (axis === "horizontal") {
      first.height = frame.height / 2;
      second.y = frame.y + frame.height / 2;
      second.height = frame.height / 2;
    } else {
      first.width = frame.width / 2;
      second.x = frame.x + frame.width / 2;
      second.width = frame.width / 2;
    }
    commit((current) => [...current.slice(0, selected), first, second, ...current.slice(selected + 1)].map((item, index) => ({ ...item, order: index + 1 })));
    setSelectedFrames(new Set([selected, selected + 1]));
  }

  function mergeFrames() {
    const indices = [...selectedFrames].sort((a, b) => a - b);
    if (indices.length < 2) return;
    const selectedItems = indices.map((index) => frames[index]);
    const bounds = selectedItems.map(frameBounds);
    const left = Math.min(...bounds.map((item) => item.x));
    const top = Math.min(...bounds.map((item) => item.y));
    const right = Math.max(...bounds.map((item) => item.x + item.width));
    const bottom = Math.max(...bounds.map((item) => item.y + item.height));
    const insertAt = indices[0];
    const merged = defaultFrame(insertAt + 1, { name: "Merged panel", x: left, y: top, width: right - left, height: bottom - top, source: "manual" });
    commit((current) => {
      const remaining = current.filter((_, index) => !selectedFrames.has(index));
      remaining.splice(insertAt, 0, merged);
      return remaining.map((frame, index) => ({ ...frame, order: index + 1 }));
    });
    setSelected(insertAt);
    setSelectedFrames(new Set([insertAt]));
  }

  function toggleSelected(index: number) {
    setSelected(index);
    setSelectedFrames((current) => {
      const next = new Set(current);
      if (next.has(index) && next.size > 1) next.delete(index); else next.add(index);
      return next;
    });
  }

  function dropOrder(target: number) {
    if (draggedOrder === null || draggedOrder === target) return;
    commit((current) => {
      const next = [...current];
      const [moved] = next.splice(draggedOrder, 1);
      next.splice(target, 0, moved);
      return next.map((frame, index) => ({ ...frame, order: index + 1, source: frame.source === "detected" ? "manual_edited" : frame.source }));
    });
    setSelected(target);
    setSelectedFrames(new Set([target]));
    setDraggedOrder(null);
  }

  function reorder(direction: -1 | 1) {
    const target = selected + direction;
    if (target < 0 || target >= frames.length) return;
    commit((current) => {
      const next = [...current];
      [next[selected], next[target]] = [next[target], next[selected]];
      return next.map((frame, index) => ({ ...frame, order: index + 1, source: frame.source === "detected" ? "manual_edited" : "manual" }));
    });
    setSelected(target);
  }

  async function save() {
    setSaving(true);
    setError("");
    try {
      await onSave(framesRef.current.map((frame, index) => ({ ...frame, order: index + 1, source: frame.source === "detected" ? "manual_edited" : frame.source })));
      setDirty(false);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not save frames.");
    } finally {
      setSaving(false);
    }
  }

  function updateActive(patch: Partial<FocusFrame>) {
    commit((current) => current.map((frame, index) => index === selected ? { ...frame, ...patch, source: frame.source === "detected" ? "manual_edited" : "manual" } : frame));
  }

  function updatePoint(pointIndex: number, axis: "x" | "y", value: number) {
    if (!active) return;
    const polygon = active.polygon.map((point, index) => index === pointIndex ? { ...point, [axis]: clamp(value, 0, 1) } : point);
    updateActive({ polygon });
  }

  function addPoint() {
    if (!active || active.polygon.length >= 100) return;
    const last = active.polygon.at(-1) ?? { x: 0.5, y: 0.5 };
    updateActive({ polygon: [...active.polygon, { x: clamp(last.x + 0.03, 0, 1), y: clamp(last.y + 0.03, 0, 1) }] });
  }

  function removePoint(index: number) {
    if (!active || active.polygon.length <= 3) return;
    updateActive({ polygon: active.polygon.filter((_, pointIndex) => pointIndex !== index) });
  }

  function undoChange() {
    const previous = undo.at(-1);
    if (!previous) return;
    setRedo((history) => [...history, frames]);
    setUndo((history) => history.slice(0, -1));
    setFrames(previous);
    setDirty(true);
  }

  function redoChange() {
    const next = redo.at(-1);
    if (!next) return;
    setUndo((history) => [...history, frames]);
    setRedo((history) => history.slice(0, -1));
    setFrames(next);
    setDirty(true);
  }

  function done() {
    if (dirty && !window.confirm("Discard unsaved frame changes?")) return;
    onCancel();
  }

  async function detect() {
    const hasManual = frames.some((frame) => frame.source === "manual" || frame.source === "manual_edited");
    if (hasManual && !window.confirm("Automatic detection will replace your manual frames on this page. Continue?")) return;
    setDetecting(true);
    setError("");
    try {
      const detected = await onDetect(hasManual);
      setFrames(detected);
      setSelected(0);
      setSelectedFrames(new Set([0]));
      setDirty(false);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not detect panels.");
    } finally {
      setDetecting(false);
    }
  }

  async function approve() {
    setApproving(true);
    setError("");
    try {
      if (dirty) await save();
      await onApprove(reviewStatus !== "approved");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not update review status.");
    } finally {
      setApproving(false);
    }
  }

  const active = frames[selected];
  return (
    <section className="frame-editor-shell">
      <aside className="editor-tools">
        <div><span className="tool-kicker">Frame editor</span><strong>Correct the reading path</strong></div>
        <p>Choose a frame, then drag its center to move it or its corner to resize it. Changes auto-save after you finish editing.</p>
        <div className="editor-actions">
          <button className="detect-action" disabled={detecting} onClick={() => void detect()}>{detecting ? "Detecting..." : "Auto detect"}</button>
          <button disabled={active?.shapeType !== "rectangle"} onClick={() => splitFrame("horizontal")}>Split ↕</button>
          <button disabled={active?.shapeType !== "rectangle"} onClick={() => splitFrame("vertical")}>Split ↔</button>
          <button disabled={selectedFrames.size < 2} onClick={mergeFrames}>Merge</button>
          <button onClick={addFrame}>+ Rectangle</button>
          <button onClick={addPolygon}>+ Polygon</button>
          <button onClick={addFullPage}>Full page</button>
          <button onClick={duplicateFrame}>Duplicate</button>
          <button disabled={frames.length <= 1} onClick={removeFrame}>Delete</button>
          <button disabled={selected === 0} onClick={() => reorder(-1)}>Earlier</button>
          <button disabled={selected === frames.length - 1} onClick={() => reorder(1)}>Later</button>
          <button disabled={!undo.length} onClick={undoChange}>Undo</button>
          <button disabled={!redo.length} onClick={redoChange}>Redo</button>
        </div>
        <ol className="frame-list">
          {frames.map((frame, index) => <li draggable key={index} onDragStart={() => setDraggedOrder(index)} onDragOver={(event) => event.preventDefault()} onDrop={() => dropOrder(index)}><button className={selected === index ? "active" : ""} onClick={() => toggleSelected(index)}><span className={`selection-dot${selectedFrames.has(index) ? " selected" : ""}`} />Frame {index + 1}{frame.confidence !== undefined && <small className={frame.confidence < .5 ? "low" : frame.confidence < .85 ? "review" : "likely"}>{Math.round(frame.confidence * 100)}%</small>}</button></li>)}
        </ol>
        <section className={`review-card status-${reviewStatus}`}>
          <div><span>Page review</span><strong>{reviewStatus.replaceAll("_", " ")}</strong></div>
          <dl><div><dt>Coverage</dt><dd>{Math.round((detectionReport.coverage || 0) * 100)}%</dd></div><div><dt>Frames</dt><dd>{detectionReport.panelCount || frames.length}</dd></div></dl>
          {!!detectionReport.warnings?.length && <ul>{detectionReport.warnings.map((warning) => <li key={warning}>{warning.replaceAll("_", " ")}</li>)}</ul>}
          <button disabled={approving || dirty} onClick={() => void approve()}>{approving ? "Updating..." : reviewStatus === "approved" ? "Unapprove page" : "Approve page"}</button>
          <div className="export-actions"><button onClick={() => onExport("yolo")}>Export YOLO</button><button onClick={() => onExport("coco")}>Export COCO</button></div>
        </section>
        {active && <div className="frame-properties">
          <label>Name<input value={active.name} onChange={(event) => updateActive({ name: event.target.value })} /></label>
          <label>Type<select value={active.frameType} onChange={(event) => updateActive({ frameType: event.target.value as FocusFrame["frameType"] })}><option value="panel">Panel</option><option value="full_page">Full page</option><option value="focus">Focus</option><option value="speech">Speech</option><option value="object">Object</option></select></label>
          <label>Fit<select value={active.fitMode} onChange={(event) => updateActive({ fitMode: event.target.value as FocusFrame["fitMode"] })}><option value="contain">Contain</option><option value="cover">Cover</option></select></label>
          <label>Padding <output>{active.paddingPercent}%</output><input type="range" min="0" max="50" value={active.paddingPercent} onChange={(event) => updateActive({ paddingPercent: Number(event.target.value) })} /></label>
          <label>Mask <output>{Math.round(active.maskOpacity * 100)}%</output><input type="range" min="0" max="1" step="0.05" value={active.maskOpacity} onChange={(event) => updateActive({ maskOpacity: Number(event.target.value) })} /></label>
          <label>Transition <output>{active.transitionDurationMs} ms</output><input type="range" min="0" max="1200" step="25" value={active.transitionDurationMs} onChange={(event) => updateActive({ transitionDurationMs: Number(event.target.value) })} /></label>
          <label className="check-label"><input type="checkbox" checked={active.isEnabled} onChange={(event) => updateActive({ isEnabled: event.target.checked })} /> Enabled</label>
          {active.confidence !== undefined && <div className="detection-confidence"><span>AI confidence</span><strong>{Math.round(active.confidence * 100)}%</strong><small>{active.confidence >= .85 ? "Likely correct" : active.confidence >= .5 ? "Review recommended" : "Low confidence"}{active.modelVersion ? ` · ${active.modelVersion}` : ""}</small></div>}
          {active.shapeType === "polygon" && <fieldset className="polygon-points"><legend>Polygon points</legend>{active.polygon.map((point, index) => <div key={index}><span>{index + 1}</span><input aria-label={`Point ${index + 1} X`} type="number" min="0" max="1" step="0.01" value={point.x} onChange={(event) => updatePoint(index, "x", Number(event.target.value))} /><input aria-label={`Point ${index + 1} Y`} type="number" min="0" max="1" step="0.01" value={point.y} onChange={(event) => updatePoint(index, "y", Number(event.target.value))} /><button disabled={active.polygon.length <= 3} onClick={() => removePoint(index)}>×</button></div>)}<button onClick={addPoint}>+ Point</button></fieldset>}
        </div>}
        <div className="save-status">{saving ? "Saving..." : dirty ? "Unsaved changes" : "Saved"}</div>
        {error && <p className="error" role="alert">{error}</p>}
        <div className="editor-commit"><button onClick={done}>Done</button><button className="save" disabled={saving || !dirty} onClick={() => void save()}>{saving ? "Saving..." : "Save now"}</button></div>
      </aside>
      <div className="editor-canvas-wrap">
        <div className="editor-canvas" onPointerMove={moveDrag} onPointerUp={() => setDrag(null)} onPointerCancel={() => setDrag(null)}>
          <img src={image} alt="Page being framed" draggable={false} />
          <FrameOverlay frames={frames} selected={selected} onSelect={setSelected} />
          {active && active.shapeType === "rectangle" && (
            <div className="editable-frame" style={{ left: `${frameBounds(active).x * 100}%`, top: `${frameBounds(active).y * 100}%`, width: `${frameBounds(active).width * 100}%`, height: `${frameBounds(active).height * 100}%` }}>
              <button className="move-handle" aria-label="Move selected frame" onPointerDown={(event) => startDrag(event, "move")} />
              {(["nw", "n", "ne", "e", "se", "s", "sw", "w"] as const).map((side) => <button className={`resize-handle handle-${side}`} aria-label={`Resize frame from ${side}`} key={side} onPointerDown={(event) => startDrag(event, side)} />)}
            </div>
          )}
          {active && active.shapeType === "polygon" && (
            <div className="editable-polygon">
              <svg viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden="true">
                <polygon points={active.polygon.map((point) => `${point.x * 100},${point.y * 100}`).join(" ")} />
              </svg>
              <button
                className="polygon-move-handle"
                aria-label="Move complete polygon frame"
                style={{ left: `${(frameBounds(active).x + frameBounds(active).width / 2) * 100}%`, top: `${(frameBounds(active).y + frameBounds(active).height / 2) * 100}%` }}
                onPointerDown={(event) => startDrag(event, "polygon-move")}
              />
              {active.polygon.map((point, index) => (
                <button
                  className="polygon-vertex-handle"
                  aria-label={`Move polygon point ${index + 1}`}
                  key={index}
                  style={{ left: `${point.x * 100}%`, top: `${point.y * 100}%` }}
                  onPointerDown={(event) => startDrag(event, "vertex", index)}
                >{index + 1}</button>
              ))}
            </div>
          )}
        </div>
      </div>
    </section>
  );
}

function clamp(value: number, minimum: number, maximum: number) {
  return Math.max(minimum, Math.min(maximum, value));
}
