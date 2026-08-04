import { ChangeEvent, CSSProperties, useEffect, useRef, useState } from "react";
import { FrameOverlay } from "./reader/debug/FrameOverlay";
import { FocusFrame, calculateCamera } from "./reader/engine/camera";
import { FrameEditor } from "./reader/editor/FrameEditor";

const API = "/api/v1";

interface Comic {
  id: string;
  title: string;
  status: string;
  progress: number;
  phase: string;
  error_message?: string;
  page_count: number;
  cover_url?: string;
}

interface Page {
  number: number;
  width: number;
  height: number;
  media_type: string;
  image_url: string;
  frames: FocusFrame[];
  panels?: FocusFrame[];
  revision: number;
  frameSetupComplete: boolean;
}

type ReaderMode = "panel" | "page" | "book";

interface SavedProgress {
  page: number;
  frame: number;
  mode: "panel" | "page" | "vertical";
  direction: "ltr" | "rtl";
}

interface APIError {
  error?: { message?: string };
}

export function App() {
  const [comics, setComics] = useState<Comic[]>([]);
  const [comic, setComic] = useState<Comic | null>(null);
  const [pages, setPages] = useState<Page[]>([]);
  const [pageIndex, setPageIndex] = useState(0);
  const [panelIndex, setPanelIndex] = useState(-1);
  const [readerMode, setReaderMode] = useState<ReaderMode>("page");
  const [direction, setDirection] = useState<"ltr" | "rtl">("ltr");
  const [editingFrames, setEditingFrames] = useState(false);
  const [showFrameDebug, setShowFrameDebug] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [importProgress, setImportProgress] = useState(0);
  const [importPhase, setImportPhase] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    void loadComics();
  }, []);

  useEffect(() => {
    function navigate(event: KeyboardEvent) {
      const target = event.target as HTMLElement | null;
      if (target?.matches("input, textarea, select, [contenteditable='true']")) return;
      if (event.key === "ArrowLeft") {
        event.preventDefault();
        navigateReader(direction === "ltr" ? -1 : 1);
      }
      if (event.key === "ArrowRight" || event.key === " ") {
        event.preventDefault();
        navigateReader(event.key === " " ? event.shiftKey ? -1 : 1 : direction === "ltr" ? 1 : -1);
      }
      if (event.key.toLowerCase() === "f") void document.documentElement.requestFullscreen?.();
      if (event.key.toLowerCase() === "m") cycleMode();
      if (event.key === "Escape" && document.fullscreenElement) void document.exitFullscreen();
    }
    window.addEventListener("keydown", navigate);
    return () => window.removeEventListener("keydown", navigate);
  }, [direction, pageIndex, panelIndex, pages, readerMode]);

  useEffect(() => {
    if (!comic || !pages.length) return;
    const mode = readerMode === "book" ? "vertical" : readerMode;
    const frame = panelIndex >= 0 ? activeFrames(pages[pageIndex])[panelIndex]?.order ?? 1 : 1;
    const timer = window.setTimeout(() => {
      const progress = { page: pageIndex + 1, frame, mode, direction };
      localStorage.setItem(`panel-reader:${comic.id}`, JSON.stringify(progress));
      void fetch(`${API}/comics/${comic.id}/progress`, { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(progress) });
    }, 600);
    return () => window.clearTimeout(timer);
  }, [comic, direction, pageIndex, panelIndex, pages, readerMode]);

  async function loadComics() {
    try {
      const response = await fetch(`${API}/comics`);
      if (!response.ok) throw new Error("Could not load your comics.");
      setComics(await response.json() as Comic[]);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not connect to the API.");
    }
  }

  async function openComic(selected: Comic) {
    setError("");
    const response = await fetch(`${API}/comics/${selected.id}/pages`);
    if (!response.ok) {
      setError("Could not load comic pages.");
      return;
    }
    const loaded = await response.json() as Page[];
    setPages(loaded.map((page) => ({ ...page, frames: [...(page.frames?.length ? page.frames : page.panels ?? [])].sort((a, b) => a.order - b.order) })));
    setComic(selected);
    let restored: SavedProgress | null = null;
    try {
      const response = await fetch(`${API}/comics/${selected.id}/progress`);
      if (response.ok) restored = await response.json() as SavedProgress;
      if (!restored) restored = JSON.parse(localStorage.getItem(`panel-reader:${selected.id}`) || "null") as SavedProgress | null;
    } catch { /* Local progress remains a safe fallback. */ }
    const restoredPage = Math.max(0, Math.min(loaded.length - 1, (restored?.page ?? 1) - 1));
    setPageIndex(restoredPage);
    setReaderMode(restored?.mode === "vertical" ? "book" : restored?.mode ?? "page");
    setDirection(restored?.direction ?? "ltr");
    const frameIndex = restored?.mode === "panel" ? Math.max(0, activeFrames(loaded[restoredPage]).findIndex((frame) => frame.order === restored?.frame)) : -1;
    setPanelIndex(frameIndex);
  }

  function selectMode(mode: ReaderMode) {
    setReaderMode(mode);
    setPanelIndex(mode === "panel" ? Math.max(0, panelIndex) : -1);
  }

  function cycleMode() {
    const modes: ReaderMode[] = ["panel", "page", "book"];
    selectMode(modes[(modes.indexOf(readerMode) + 1) % modes.length]);
  }

  function navigateReader(direction: -1 | 1) {
    if (readerMode === "book") {
      window.scrollBy({ top: direction * window.innerHeight * 0.85, behavior: "smooth" });
      return;
    }
    if (readerMode === "page") {
      setPageIndex((value) => Math.max(0, Math.min(pages.length - 1, value + direction)));
      return;
    }
    const panelCount = activeFrames(pages[pageIndex]).length;
    if (direction === 1) {
      if (panelIndex < panelCount - 1) {
        setPanelIndex((value) => value + 1);
      } else if (pageIndex < pages.length - 1) {
        setPageIndex((value) => value + 1);
        setPanelIndex(0);
      }
      return;
    }
    if (panelIndex >= 0) {
      setPanelIndex((value) => value - 1);
    } else if (pageIndex > 0) {
      const previousPage = pages[pageIndex - 1];
      setPageIndex((value) => value - 1);
      setPanelIndex(Math.max(0, activeFrames(previousPage).length - 1));
    }
  }

  async function upload(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    if (!file) return;
    setUploading(true);
    setImportProgress(0);
    setImportPhase("Uploading");
    setError("");
    const data = new FormData();
    data.append("file", file);
    try {
      const imported = await uploadComic(data, (percent) => {
        setImportProgress(Math.round(percent * 0.3));
      });
      setComics((current) => [imported, ...current]);
      const ready = await waitForImport(imported.id);
      setComics((current) => current.map((item) => item.id === ready.id ? ready : item));
      await openComic(ready);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Import failed.");
    } finally {
      setUploading(false);
      setImportProgress(0);
      setImportPhase("");
      event.target.value = "";
    }
  }

  function uploadComic(data: FormData, onProgress: (percent: number) => void): Promise<Comic> {
    return new Promise((resolve, reject) => {
      const request = new XMLHttpRequest();
      request.open("POST", `${API}/comics`);
      request.responseType = "json";
      request.upload.addEventListener("progress", (progressEvent) => {
        if (progressEvent.lengthComputable) onProgress((progressEvent.loaded / progressEvent.total) * 100);
      });
      request.addEventListener("load", () => {
        if (request.status >= 200 && request.status < 300) {
          resolve(request.response as Comic);
          return;
        }
        const body = request.response as APIError | null;
        reject(new Error(body?.error?.message ?? "Import failed."));
      });
      request.addEventListener("error", () => reject(new Error("Could not connect to the API.")));
      request.send(data);
    });
  }

  async function waitForImport(id: string): Promise<Comic> {
    for (;;) {
      const response = await fetch(`${API}/comics/${id}`);
      if (!response.ok) throw new Error("Could not read import progress.");
      const current = await response.json() as Comic;
      const phase = current.phase === "publishing" ? "Finishing" : current.phase === "queued" ? "Preparing" : titleCase(current.phase);
      setImportPhase(phase);
      setImportProgress(Math.min(100, 30 + Math.round(current.progress * 0.7)));
      if (current.status === "ready") return current;
      if (current.status === "failed") throw new Error(current.error_message || "Import failed.");
      await new Promise((resolve) => window.setTimeout(resolve, 400));
    }
  }

  if (comic && pages.length > 0) {
    const page = pages[pageIndex];
    const positionLabel = readerMode === "panel"
      ? `Page ${pageIndex + 1} · Frame ${panelIndex + 1}/${Math.max(1, activeFrames(page).length)}`
      : readerMode === "book" ? `${pages.length} pages` : `${pageIndex + 1} / ${pages.length}`;
    if (editingFrames) {
      return <FrameEditor image={page.image_url} initialFrames={page.frames} onCancel={() => setEditingFrames(false)} onSave={async (frames) => {
        const response = await fetch(`${API}/comics/${comic.id}/pages/${page.number}/frames`, {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ revision: page.revision, frames }),
        });
        if (!response.ok) {
          const body = await response.json() as APIError;
          throw new Error(body.error?.message ?? "Could not save frames.");
        }
        const saved = await response.json() as { revision: number; frames: FocusFrame[] };
        setPages((current) => current.map((item, index) => index === pageIndex ? { ...item, frames: saved.frames, revision: saved.revision, frameSetupComplete: saved.frames.some((frame) => frame.isEnabled) } : item));
      }} />;
    }
    return (
      <main className={`reader-shell mode-${readerMode}`}>
        <header className="reader-bar">
          <button className="text-button" onClick={() => setComic(null)}>Library</button>
          <div className="reader-title">
            <strong>{comic.title}</strong>
            <span>{positionLabel}</span>
          </div>
          <div className="reader-actions">
            {readerMode === "panel" && <button className={`debug-button${showFrameDebug ? " active" : ""}`} onClick={() => setShowFrameDebug((value) => !value)}>Outline</button>}
            {readerMode === "panel" && <button className="debug-button" onClick={() => setEditingFrames(true)}>Edit frames</button>}
            <button className="debug-button" onClick={() => setDirection((value) => value === "ltr" ? "rtl" : "ltr")}>{direction.toUpperCase()}</button>
            <div className="mode-switcher" aria-label="Reading mode">
              {(["panel", "page", "book"] as ReaderMode[]).map((mode) => (
                <button className={readerMode === mode ? "active" : ""} key={mode} onClick={() => selectMode(mode)}>{mode === "book" ? "Vertical" : titleCase(mode)}</button>
              ))}
            </div>
            <button className="text-button fullscreen-button" onClick={() => void document.documentElement.requestFullscreen?.()}>Fullscreen</button>
          </div>
        </header>
        {readerMode === "book" ? (
          <section className="book-stage">
            {pages.map((bookPage) => <img key={bookPage.number} src={bookPage.image_url} alt={`${comic.title}, page ${bookPage.number}`} loading={bookPage.number < 3 ? "eager" : "lazy"} />)}
          </section>
        ) : readerMode === "panel" ? (
          <PanelStage comic={comic} page={page} panelIndex={panelIndex} showDebug={showFrameDebug} onPrevious={() => navigateReader(-1)} onNext={() => navigateReader(1)} />
        ) : (
          <section className="stage">
            {pageIndex > 0 && <link rel="preload" as="image" href={pages[pageIndex - 1].image_url} />}
            {pageIndex + 1 < pages.length && <link rel="preload" as="image" href={pages[pageIndex + 1].image_url} />}
            <img src={page.image_url} alt={`${comic.title}, page ${page.number}`} />
          </section>
        )}
        {readerMode !== "book" && (
          <nav className="reader-nav" aria-label="Reader navigation">
            <button onClick={() => navigateReader(-1)}>Previous</button>
            <div className="progress"><i style={{ width: `${readerProgress(readerMode, pageIndex, panelIndex, pages) * 100}%` }} /></div>
            <button onClick={() => navigateReader(1)}>Next</button>
          </nav>
        )}
      </main>
    );
  }

  return (
    <main className="library-shell">
      <header className="masthead">
        <p className="eyebrow">Local comic archive</p>
        <h1>Panel<br /><em>Reader</em></h1>
        <p className="intro">Open a CBZ, CBR, or PDF, then read by guided frames, single pages, or the complete scrolling book.</p>
        <label
          className={`upload-button${uploading ? " is-importing" : ""}`}
          style={{ "--border-progress": `${importProgress * 3.6}deg` } as CSSProperties}
        >
          <span>{uploading ? `${importPhase} ${importProgress}%` : "Import a comic"}</span>
          <input
            type="file"
            accept=".cbz,.cbr,.pdf,application/vnd.comicbook+zip,application/vnd.comicbook-rar,application/pdf"
            disabled={uploading}
            onChange={upload}
          />
        </label>
        {error && <p className="error" role="alert">{error}</p>}
      </header>
      <section className="shelf" aria-label="Comic library">
        <div className="section-heading"><span>Your shelf</span><span>{comics.length} volumes</span></div>
        {comics.length === 0 ? (
          <div className="empty"><span>01</span><p>Your first story starts with an import.</p></div>
        ) : (
          <div className="comic-grid">
            {comics.map((item, index) => (
              <button className="comic-card" key={item.id} onClick={() => void openComic(item)}>
                {item.cover_url && <img className="card-cover" src={item.cover_url} alt="" loading="lazy" />}
                <span className="card-shade" />
                <span className="card-number">{String(index + 1).padStart(2, "0")}</span>
                <span className="card-copy"><span className="card-title">{item.title}</span><span className="card-meta">{item.page_count} pages</span></span>
              </button>
            ))}
          </div>
        )}
      </section>
    </main>
  );
}

function titleCase(value: string): string {
  return value ? value[0].toUpperCase() + value.slice(1) : "Processing";
}

function PanelStage({ comic, page, panelIndex, showDebug, onPrevious, onNext }: { comic: Comic; page: Page; panelIndex: number; showDebug: boolean; onPrevious: () => void; onNext: () => void }) {
  const stageRef = useRef<HTMLElement>(null);
  const [viewport, setViewport] = useState({ width: 0, height: 0 });

  useEffect(() => {
    const stage = stageRef.current;
    if (!stage) return;
    const observer = new ResizeObserver(([entry]) => setViewport({ width: entry.contentRect.width, height: entry.contentRect.height }));
    observer.observe(stage);
    return () => observer.disconnect();
  }, []);

  const frames = activeFrames(page);
  const focus = frames[panelIndex] ?? defaultReaderFrame();
  const padding = Math.min(48, viewport.width * 0.04, viewport.height * 0.04);
  const camera = calculateCamera({ viewportWidth: viewport.width, viewportHeight: viewport.height, pageWidth: page.width, pageHeight: page.height, frame: focus, padding, maxScale: 3 });

  return (
    <section className="panel-stage" ref={stageRef}>
      <div className="page-camera" style={{ width: page.width, height: page.height, transform: `translate3d(${camera.x}px, ${camera.y}px, 0) scale(${camera.scale})`, transitionDuration: `${focus.transitionDurationMs}ms`, transitionTimingFunction: focus.easing }}>
        <img src={page.image_url} alt={`${comic.title}, page ${page.number}${panelIndex >= 0 ? `, frame ${panelIndex + 1}` : " overview"}`} />
        {focus.maskOpacity > 0 && focus.frameType !== "full_page" && <FrameMask frame={focus} page={page} />}
        {showDebug && <FrameOverlay frames={frames} selected={panelIndex} />}
      </div>
      <button className="tap-zone previous-zone" aria-label="Previous frame" onClick={onPrevious} />
      <button className="tap-zone next-zone" aria-label="Next frame" onClick={onNext} />
      <div className="frame-indicator">{`${String(panelIndex + 1).padStart(2, "0")} / ${String(frames.length).padStart(2, "0")}`}</div>
    </section>
  );
}

function readerProgress(mode: ReaderMode, pageIndex: number, panelIndex: number, pages: Page[]): number {
  if (mode === "page") return (pageIndex + 1) / pages.length;
  const completedBefore = pages.slice(0, pageIndex).reduce((sum, page) => sum + Math.max(1, activeFrames(page).length), 0);
  const total = pages.reduce((sum, page) => sum + Math.max(1, activeFrames(page).length), 0);
  return (completedBefore + panelIndex + 1) / total;
}

function activeFrames(page: Page): FocusFrame[] {
  return page.frames.filter((frame) => frame.isEnabled).sort((a, b) => a.order - b.order);
}

function defaultReaderFrame(): FocusFrame {
  return { name: "Full page fallback", order: 1, shapeType: "rectangle", frameType: "full_page", x: 0, y: 0, width: 1, height: 1, polygon: [], fitMode: "contain", paddingPercent: 2, maskOpacity: 0, transitionDurationMs: 0, easing: "linear", source: "manual", isEnabled: true };
}

function FrameMask({ frame, page }: { frame: FocusFrame; page: Page }) {
  const id = `frame-mask-${page.number}-${frame.id ?? frame.order}`;
  const cutout = frame.shapeType === "polygon"
    ? <polygon points={frame.polygon.map((point) => `${point.x * page.width},${point.y * page.height}`).join(" ")} fill="black" />
    : <rect x={frame.x * page.width} y={frame.y * page.height} width={frame.width * page.width} height={frame.height * page.height} fill="black" />;
  return <svg className="active-frame-mask" viewBox={`0 0 ${page.width} ${page.height}`} aria-hidden="true"><defs><mask id={id}><rect width={page.width} height={page.height} fill="white" />{cutout}</mask></defs><rect width={page.width} height={page.height} fill={`rgba(0,0,0,${frame.maskOpacity})`} mask={`url(#${id})`} /></svg>;
}
