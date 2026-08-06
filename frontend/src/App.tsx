import { ChangeEvent, CSSProperties, startTransition, useEffect, useRef, useState } from "react";
import { FocusFrame } from "./reader/engine/camera";
import { FrameEditor } from "./reader/editor/FrameEditor";
import { preloadImage, preloadWindow } from "./reader/preloading/images";
import { ReaderControls, ReaderSpeed, SPEEDS } from "./reader/ReaderControls";
import { ReaderStage } from "./reader/ReaderStage";

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
  contentType: "comic" | "manga" | "webtoon";
  readingDirection: "ltr" | "rtl" | "vertical";
  defaultReadingMode: "panel" | "page" | "vertical";
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
  reviewStatus: string;
  detectionReport: {
    warnings: string[];
    panelCount: number;
    coverage: number;
    averageConfidence: number;
    aiCandidateCount?: number;
    structuralCandidateCount?: number;
    recoveredPanelCount?: number;
  };
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
  const [pendingDelete, setPendingDelete] = useState<Comic | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [changingPage, setChangingPage] = useState(false);
  const [speed, setSpeed] = useState<ReaderSpeed>("normal");
  const [zoom, setZoomState] = useState(1);
  const [fit, setFit] = useState<"page" | "width">("page");
  const [pan, setPan] = useState({ x: 0, y: 0 });
  const [imageError, setImageError] = useState("");
  const [importContentType, setImportContentType] = useState<"comic" | "manga" | "webtoon">("comic");
  const changingRef = useRef(false);
  const queuedNavigation = useRef<-1 | 1 | null>(null);

  useEffect(() => {
    void loadComics();
  }, []);

  useEffect(() => {
    function navigate(event: KeyboardEvent) {
      const target = event.target as HTMLElement | null;
      if (target?.matches("input, textarea, select, [contenteditable='true']")) return;
      if (event.key === "ArrowLeft") {
        event.preventDefault();
        navigateReader(-1);
      }
      if (event.key === "ArrowRight" || event.key === " ") {
        event.preventDefault();
        navigateReader(event.key === " " && event.shiftKey ? -1 : 1);
      }
      if (event.key.toLowerCase() === "f") void document.documentElement.requestFullscreen?.();
      if (event.key.toLowerCase() === "m") cycleMode();
      if (event.key === "Home") { event.preventDefault(); void changePage(0, readerMode === "panel" ? 0 : -1); }
      if (event.key === "End") { event.preventDefault(); void changePage(pages.length - 1, readerMode === "panel" ? Math.max(0, activeFrames(pages[pages.length - 1]).length - 1) : -1); }
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

  useEffect(() => {
    if (!comic || !pages.length || readerMode === "book") return;
    preloadWindow(pages.map((page) => page.image_url), pageIndex);
  }, [comic, pageIndex, pages, readerMode]);

  useEffect(() => {
    if (changingPage) return;
    changingRef.current = false;
    const queued = queuedNavigation.current;
    queuedNavigation.current = null;
    if (queued) navigateReader(queued);
  }, [changingPage, pageIndex, panelIndex]);

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
    const preparedPages = loaded.map((page) => ({ ...page, frames: [...(page.frames?.length ? page.frames : page.panels ?? [])].sort((a, b) => a.order - b.order) }));
    if (!preparedPages.length) {
      setError("This comic has no readable page images. Re-import the source file.");
      return;
    }
    await preloadImage(preparedPages[0].image_url).catch(() => undefined);
    setPages(preparedPages);
    setComic(selected);
    let restored: SavedProgress | null = null;
    try {
      const response = await fetch(`${API}/comics/${selected.id}/progress`);
      if (response.ok) restored = await response.json() as SavedProgress;
      if (!restored) restored = JSON.parse(localStorage.getItem(`panel-reader:${selected.id}`) || "null") as SavedProgress | null;
    } catch { /* Local progress remains a safe fallback. */ }
    const restoredPage = Math.max(0, Math.min(loaded.length - 1, (restored?.page ?? 1) - 1));
    await preloadImage(preparedPages[restoredPage].image_url).catch(() => undefined);
    setPageIndex(restoredPage);
    const defaultMode = selected.defaultReadingMode === "vertical" ? "book" : selected.defaultReadingMode || "page";
    setReaderMode(restored?.mode === "vertical" ? "book" : restored?.mode ?? defaultMode);
    setDirection(restored?.direction ?? (selected.readingDirection === "rtl" ? "rtl" : "ltr"));
    const frameIndex = restored?.mode === "panel" ? Math.max(0, activeFrames(loaded[restoredPage]).findIndex((frame) => frame.order === restored?.frame)) : -1;
    setPanelIndex(frameIndex);
  }

  function selectMode(mode: ReaderMode) {
    setReaderMode(mode);
    setPanelIndex(mode === "panel" ? Math.max(0, panelIndex) : -1);
    setPan({ x: 0, y: 0 });
  }

  function cycleMode() {
    const modes: ReaderMode[] = ["panel", "page", "book"];
    selectMode(modes[(modes.indexOf(readerMode) + 1) % modes.length]);
  }

  function navigateReader(direction: -1 | 1) {
    if (changingRef.current) { queuedNavigation.current = direction; return; }
    if (readerMode === "book") {
      window.scrollBy({ top: direction * window.innerHeight * 0.85, behavior: "smooth" });
      return;
    }
    if (readerMode === "page") {
      const target = Math.max(0, Math.min(pages.length - 1, pageIndex + direction));
      if (target !== pageIndex) void changePage(target, -1);
      return;
    }
    const panelCount = activeFrames(pages[pageIndex]).length;
    if (direction === 1) {
      if (panelIndex < panelCount - 1) {
        setPanelIndex((value) => value + 1);
        setPan({ x: 0, y: 0 });
      } else if (pageIndex < pages.length - 1) {
        void changePage(pageIndex + 1, 0);
      }
      return;
    }
    if (panelIndex > 0) {
      setPanelIndex((value) => value - 1);
      setPan({ x: 0, y: 0 });
    } else if (pageIndex > 0) {
      const previousPage = pages[pageIndex - 1];
      void changePage(pageIndex - 1, Math.max(0, activeFrames(previousPage).length - 1));
    }
  }

  async function changePage(targetPage: number, targetFrame: number) {
    if (targetPage < 0 || targetPage >= pages.length || changingRef.current) return;
    changingRef.current = true;
    setChangingPage(true);
    setImageError("");
    try {
      await preloadImage(pages[targetPage].image_url);
    } catch (cause) {
      setImageError(cause instanceof Error ? cause.message : "The page image could not be loaded.");
      changingRef.current = false;
      setChangingPage(false);
      return;
    }
    startTransition(() => {
      setPageIndex(targetPage);
      setPanelIndex(targetFrame);
      setPan({ x: 0, y: 0 });
      setChangingPage(false);
    });
  }

  function setZoom(value: number) { setZoomState(Math.max(.25, Math.min(5, value))); }

  async function changeContentType(contentType: Comic["contentType"]) {
    if (!comic) return;
    const response = await fetch(`${API}/comics/${comic.id}`, { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ contentType }) });
    if (!response.ok) {
      const body = await response.json() as APIError;
      setError(body.error?.message ?? "Could not update comic type.");
      return;
    }
    const updated = await response.json() as Comic;
    setComic(updated);
    setComics((current) => current.map((item) => item.id === updated.id ? updated : item));
    setDirection(updated.readingDirection === "rtl" ? "rtl" : "ltr");
    if (updated.defaultReadingMode === "vertical") setReaderMode("book");
    const pagesResponse = await fetch(`${API}/comics/${updated.id}/pages`);
    if (pagesResponse.ok) {
      const refreshed = await pagesResponse.json() as Page[];
      setPages(refreshed.map((page) => ({ ...page, frames: [...(page.frames?.length ? page.frames : page.panels ?? [])].sort((a, b) => a.order - b.order) })));
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
    data.append("content_type", importContentType);
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

  async function deleteComic() {
    if (!pendingDelete) return;
    setDeleting(true);
    setError("");
    try {
      const response = await fetch(`${API}/comics/${pendingDelete.id}`, { method: "DELETE" });
      if (!response.ok) {
        const body = await response.json() as APIError;
        throw new Error(body.error?.message ?? "Could not delete comic.");
      }
      setComics((current) => current.filter((item) => item.id !== pendingDelete.id));
      localStorage.removeItem(`panel-reader:${pendingDelete.id}`);
      setPendingDelete(null);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not delete comic.");
    } finally {
      setDeleting(false);
    }
  }

  if (comic && pages.length > 0) {
    const page = pages[pageIndex];
    const positionLabel = readerMode === "panel"
      ? `Page ${pageIndex + 1} · Frame ${panelIndex + 1}/${Math.max(1, activeFrames(page).length)}`
      : readerMode === "book" ? `${pages.length} pages` : `${pageIndex + 1} / ${pages.length}`;
    if (editingFrames) {
      return <FrameEditor key={page.number} image={page.image_url} initialFrames={page.frames} reviewStatus={page.reviewStatus || "unreviewed"} detectionReport={page.detectionReport || { warnings: [], panelCount: page.frames.length, coverage: 0, averageConfidence: 0 }} onCancel={() => setEditingFrames(false)} onExport={(format) => { window.location.href = `${API}/comics/${comic.id}/training-export?format=${format}`; }} onApprove={async (approved) => {
        const response = await fetch(`${API}/comics/${comic.id}/pages/${page.number}/${approved ? "approve" : "unapprove"}`, { method: "POST" });
        if (!response.ok) {
          const body = await response.json() as APIError;
          throw new Error(body.error?.message ?? "Could not update page approval.");
        }
        const result = await response.json() as { reviewStatus: string };
        setPages((current) => current.map((item, index) => index === pageIndex ? { ...item, reviewStatus: result.reviewStatus } : item));
      }} onDetect={async (reset) => {
        const response = await fetch(`${API}/comics/${comic.id}/pages/${page.number}/detect${reset ? "?reset=true" : ""}`, { method: "POST" });
        if (!response.ok) {
          const body = await response.json() as APIError;
          throw new Error(body.error?.message ?? "Could not detect panels.");
        }
        const detected = await response.json() as { revision: number; frames: FocusFrame[]; reviewStatus: string; detectionReport: Page["detectionReport"] };
        setPages((current) => current.map((item, index) => index === pageIndex ? { ...item, frames: detected.frames, revision: detected.revision, frameSetupComplete: true, reviewStatus: detected.reviewStatus, detectionReport: detected.detectionReport } : item));
        setPanelIndex(0);
        return detected.frames;
      }} onSave={async (frames) => {
        const response = await fetch(`${API}/comics/${comic.id}/pages/${page.number}/frames`, {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ revision: page.revision, frames }),
        });
        if (!response.ok) {
          const body = await response.json() as APIError;
          throw new Error(body.error?.message ?? "Could not save frames.");
        }
        const saved = await response.json() as { revision: number; frames: FocusFrame[]; reviewStatus: string; detectionReport: Page["detectionReport"] };
        setPages((current) => current.map((item, index) => index === pageIndex ? { ...item, frames: saved.frames, revision: saved.revision, frameSetupComplete: saved.frames.some((frame) => frame.isEnabled), reviewStatus: saved.reviewStatus, detectionReport: saved.detectionReport } : item));
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
            <select className="content-type-control" aria-label="Comic type" value={comic.contentType || "comic"} onChange={(event) => void changeContentType(event.target.value as Comic["contentType"])}><option value="comic">Comic</option><option value="manga">Manga</option><option value="webtoon">Webtoon</option></select>
            <div className="mode-switcher" aria-label="Reading mode">
              {(["panel", "page", "book"] as ReaderMode[]).map((mode) => (
                <button className={readerMode === mode ? "active" : ""} key={mode} onClick={() => selectMode(mode)}>{mode === "book" ? "Vertical" : titleCase(mode)}</button>
              ))}
            </div>
            <button className="text-button fullscreen-button" onClick={() => void document.documentElement.requestFullscreen?.()}>Fullscreen</button>
          </div>
        </header>
        {readerMode !== "book" && <ReaderControls page={pageIndex} total={pages.length} speed={speed} zoom={zoom} pageMode={readerMode === "page"} onPage={(index) => void changePage(index, readerMode === "panel" ? 0 : -1)} onSpeed={setSpeed} onZoom={setZoom} onFit={(value) => { setFit(value); setZoomState(1); setPan({ x: 0, y: 0 }); }} />}
        {imageError && <div className="reader-image-error" role="alert">{imageError}<button onClick={() => void changePage(pageIndex, panelIndex)}>Retry</button></div>}
        {readerMode === "book" ? (
          <section className="book-stage">
            {pages.map((bookPage) => <img key={bookPage.number} src={bookPage.image_url} alt={`${comic.title}, page ${bookPage.number}`} loading={bookPage.number < 3 ? "eager" : "lazy"} />)}
          </section>
        ) : readerMode === "panel" ? (
          <ReaderStage title={comic.title} page={page} mode="panel" panelIndex={panelIndex} showDebug={showFrameDebug} zoom={zoom} pan={pan} fit={fit} frameDuration={SPEEDS[speed].frame} pageDuration={SPEEDS[speed].page} onZoom={setZoom} onPan={setPan} onPrevious={() => navigateReader(-1)} onNext={() => navigateReader(1)} onFailure={setImageError} />
        ) : (
          <ReaderStage title={comic.title} page={page} mode="page" panelIndex={-1} showDebug={false} zoom={zoom} pan={pan} fit={fit} frameDuration={SPEEDS[speed].frame} pageDuration={SPEEDS[speed].page} onZoom={setZoom} onPan={setPan} onPrevious={() => navigateReader(-1)} onNext={() => navigateReader(1)} onFailure={setImageError} />
        )}
        {readerMode !== "book" && (
          <nav className="reader-nav" aria-label="Reader navigation">
            <button disabled={changingPage} onClick={() => navigateReader(-1)}>Previous</button>
            <div className="progress"><i style={{ width: `${readerProgress(readerMode, pageIndex, panelIndex, pages) * 100}%` }} /></div>
            <button disabled={changingPage} onClick={() => navigateReader(1)}>{changingPage ? "Loading..." : "Next"}</button>
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
        <label className="import-type">Reading format<select value={importContentType} disabled={uploading} onChange={(event) => setImportContentType(event.target.value as typeof importContentType)}><option value="comic">Comic - left to right</option><option value="manga">Manga - right to left</option><option value="webtoon">Webtoon - vertical</option></select></label>
        {error && <p className="error" role="alert">{error}</p>}
      </header>
      <section className="shelf" aria-label="Comic library">
        <div className="section-heading"><span>Your shelf</span><span>{comics.length} volumes</span></div>
        {comics.length === 0 ? (
          <div className="empty"><span>01</span><p>Your first story starts with an import.</p></div>
        ) : (
          <div className="comic-grid">
            {comics.map((item, index) => (
              <article className="comic-card" key={item.id}>
                <button className="card-open" onClick={() => void openComic(item)}>
                  {item.cover_url && <img className="card-cover" src={item.cover_url} alt="" loading="lazy" />}
                  <span className="card-shade" />
                  <span className="card-number">{String(index + 1).padStart(2, "0")}</span>
                  <span className="card-copy"><span className="card-title">{item.title}</span><span className="card-meta">{item.page_count} pages</span></span>
                </button>
                <button className="card-delete" aria-label={`Delete ${item.title}`} onClick={() => setPendingDelete(item)}>×</button>
              </article>
            ))}
          </div>
        )}
      </section>
      {pendingDelete && (
        <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget && !deleting) setPendingDelete(null); }}>
          <section className="delete-dialog" role="alertdialog" aria-modal="true" aria-labelledby="delete-title" aria-describedby="delete-description">
            <button className="dialog-close" aria-label="Close delete warning" disabled={deleting} onClick={() => setPendingDelete(null)}>×</button>
            <p className="tool-kicker">Delete comic</p>
            <h2 id="delete-title">Remove this comic?</h2>
            <p id="delete-description"><strong>{pendingDelete.title}</strong> and its imported pages, frames, and reading progress will be permanently deleted.</p>
            <div className="dialog-actions">
              <button disabled={deleting} onClick={() => setPendingDelete(null)}>Close</button>
              <button className="danger" disabled={deleting} onClick={() => void deleteComic()}>{deleting ? "Deleting..." : "Delete"}</button>
            </div>
          </section>
        </div>
      )}
    </main>
  );
}

function titleCase(value: string): string {
  return value ? value[0].toUpperCase() + value.slice(1) : "Processing";
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
