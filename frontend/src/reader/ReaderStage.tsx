import { PointerEvent, WheelEvent, useEffect, useLayoutEffect, useRef, useState } from "react";
import { FocusFrame, calculateCamera } from "./engine/camera";
import { FrameOverlay } from "./debug/FrameOverlay";

export interface ReaderPage { number: number; width: number; height: number; image_url: string; frames: FocusFrame[] }

interface Props {
  title: string; page: ReaderPage; mode: "page" | "panel"; panelIndex: number; showDebug: boolean;
  zoom: number; pan: { x: number; y: number }; fit: "page" | "width"; frameDuration: number; pageDuration: number;
  onZoom: (value: number) => void; onPan: (value: { x: number; y: number }) => void;
  onPrevious: () => void; onNext: () => void; onFailure: (message: string) => void;
}

export function ReaderStage(props: Props) {
  const stageRef = useRef<HTMLElement>(null);
  const drag = useRef<{ id: number; x: number; y: number; originX: number; originY: number } | null>(null);
  const [viewport, setViewport] = useState({ width: 0, height: 0 });
  const [previous, setPrevious] = useState<ReaderPage | null>(null);
  const currentRef = useRef(props.page);
  const frames = props.page.frames.filter((frame) => frame.isEnabled).sort((a, b) => a.order - b.order);
  const focus = frames[props.panelIndex] ?? fallbackFrame();

  useLayoutEffect(() => {
    if (currentRef.current.image_url === props.page.image_url) return;
    setPrevious(currentRef.current);
    currentRef.current = props.page;
    const timer = window.setTimeout(() => setPrevious(null), props.pageDuration + 40);
    return () => window.clearTimeout(timer);
  }, [props.page, props.pageDuration]);

  useEffect(() => {
    const stage = stageRef.current;
    if (!stage) return;
    const observer = new ResizeObserver(([entry]) => setViewport({ width: entry.contentRect.width, height: entry.contentRect.height }));
    observer.observe(stage);
    return () => observer.disconnect();
  }, []);

  const padding = Math.min(48, viewport.width * .04, viewport.height * .04);
  const base = props.mode === "panel"
    ? calculateCamera({ viewportWidth: viewport.width, viewportHeight: viewport.height, pageWidth: props.page.width, pageHeight: props.page.height, frame: focus, padding, maxScale: 8 })
    : fitCamera(props.page, viewport, padding, props.fit);
  const scale = base.scale * props.zoom;
  const transform = `translate3d(${base.x + props.pan.x}px, ${base.y + props.pan.y}px, 0) scale(${scale})`;

  function wheel(event: WheelEvent) {
    event.preventDefault();
    props.onZoom(props.zoom * Math.exp(-event.deltaY * .0015));
  }
  function pointerDown(event: PointerEvent) {
    if (event.button !== 0) return;
    event.currentTarget.setPointerCapture(event.pointerId);
    drag.current = { id: event.pointerId, x: event.clientX, y: event.clientY, originX: props.pan.x, originY: props.pan.y };
  }
  function pointerMove(event: PointerEvent) {
    if (!drag.current || drag.current.id !== event.pointerId) return;
    props.onPan({ x: drag.current.originX + event.clientX - drag.current.x, y: drag.current.originY + event.clientY - drag.current.y });
  }

  return <section className={`interactive-stage ${props.mode}-stage`} ref={stageRef} onWheel={wheel} onPointerDown={pointerDown} onPointerMove={pointerMove} onPointerUp={() => { drag.current = null; }}>
    {previous && <div className="reader-layer outgoing"><img src={previous.image_url} alt="" /></div>}
    <div className={`reader-layer current${previous ? " entering" : ""}`} style={{ width: props.page.width, height: props.page.height, transform, transitionDuration: `${props.mode === "panel" ? props.frameDuration : 0}ms`, animationDuration: `${props.pageDuration}ms` }}>
      <img src={props.page.image_url} alt={`${props.title}, page ${props.page.number}`} draggable={false} onError={() => props.onFailure(`Page ${props.page.number} could not be displayed.`)} />
      {props.mode === "panel" && focus.maskOpacity > 0 && focus.frameType !== "full_page" && <FrameMask frame={focus} page={props.page} />}
      {props.mode === "panel" && props.showDebug && <FrameOverlay frames={frames} selected={props.panelIndex} />}
    </div>
    <button className="tap-zone previous-zone" aria-label="Previous" onClick={props.onPrevious} />
    <button className="tap-zone next-zone" aria-label="Next" onClick={props.onNext} />
    {props.mode === "panel" && <div className="frame-indicator">{String(props.panelIndex + 1).padStart(2, "0")} / {String(frames.length).padStart(2, "0")}</div>}
  </section>;
}

function fitCamera(page: ReaderPage, viewport: { width: number; height: number }, padding: number, fit: "page" | "width") {
  const scale = fit === "width" ? (viewport.width - padding * 2) / page.width : Math.min((viewport.width - padding * 2) / page.width, (viewport.height - padding * 2) / page.height);
  return { scale, x: (viewport.width - page.width * scale) / 2, y: (viewport.height - page.height * scale) / 2 };
}
function fallbackFrame(): FocusFrame { return { name: "Full page", order: 1, shapeType: "rectangle", frameType: "full_page", x: 0, y: 0, width: 1, height: 1, polygon: [], fitMode: "contain", paddingPercent: 2, maskOpacity: 0, transitionDurationMs: 0, easing: "linear", source: "manual", isEnabled: true }; }
function FrameMask({ frame, page }: { frame: FocusFrame; page: ReaderPage }) {
  const id = `reader-mask-${page.number}-${frame.id ?? frame.order}`;
  const hole = frame.shapeType === "polygon" ? <polygon points={frame.polygon.map((p) => `${p.x * page.width},${p.y * page.height}`).join(" ")} fill="black" /> : <rect x={frame.x * page.width} y={frame.y * page.height} width={frame.width * page.width} height={frame.height * page.height} fill="black" />;
  return <svg className="active-frame-mask" viewBox={`0 0 ${page.width} ${page.height}`}><defs><mask id={id}><rect width={page.width} height={page.height} fill="white" />{hole}</mask></defs><rect width={page.width} height={page.height} fill={`rgba(0,0,0,${frame.maskOpacity})`} mask={`url(#${id})`} /></svg>;
}
