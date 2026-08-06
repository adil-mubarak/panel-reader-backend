import { KeyboardEvent, useEffect, useState } from "react";

export type ReaderSpeed = "instant" | "fast" | "normal" | "cinematic";
export const SPEEDS: Record<ReaderSpeed, { page: number; frame: number }> = {
  instant: { page: 0, frame: 0 }, fast: { page: 180, frame: 220 }, normal: { page: 220, frame: 460 }, cinematic: { page: 250, frame: 850 },
};

interface Props {
  page: number; total: number; speed: ReaderSpeed; zoom: number; pageMode: boolean;
  onPage: (index: number) => void; onSpeed: (speed: ReaderSpeed) => void;
  onZoom: (zoom: number) => void; onFit: (fit: "page" | "width") => void;
}

export function ReaderControls({ page, total, speed, zoom, pageMode, onPage, onSpeed, onZoom, onFit }: Props) {
  const [draft, setDraft] = useState(String(page + 1));
  useEffect(() => setDraft(String(page + 1)), [page]);

  function pageKey(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "Enter") {
      onPage(Math.max(0, Math.min(total - 1, (Number.parseInt(draft, 10) || 1) - 1)));
      event.currentTarget.blur();
    } else if (event.key === "Escape") {
      setDraft(String(page + 1));
      event.currentTarget.blur();
    }
  }

  return <div className="reader-controls">
    <label>Page <input aria-label="Page number" inputMode="numeric" value={draft} onChange={(event) => setDraft(event.target.value.replace(/\D/g, ""))} onKeyDown={pageKey} onBlur={() => setDraft(String(page + 1))} /> / {total}</label>
    <label>Speed <select value={speed} onChange={(event) => onSpeed(event.target.value as ReaderSpeed)}>{Object.keys(SPEEDS).map((value) => <option key={value} value={value}>{value[0].toUpperCase() + value.slice(1)}</option>)}</select></label>
    <div className="zoom-controls" aria-label="Zoom controls">
      <button aria-label="Zoom out" onClick={() => onZoom(zoom / 1.25)}>-</button>
      <button className="zoom-value" onClick={() => onZoom(1)}>{Math.round(zoom * 100)}%</button>
      <button aria-label="Zoom in" onClick={() => onZoom(zoom * 1.25)}>+</button>
      {pageMode && <button onClick={() => onFit("page")}>Fit page</button>}
      {pageMode && <button onClick={() => onFit("width")}>Fit width</button>}
    </div>
  </div>;
}
