export interface FocusFrame {
  id?: number;
  name: string;
  order: number;
  shapeType: "rectangle" | "polygon";
  frameType: "full_page" | "panel" | "focus" | "speech" | "object";
  x: number;
  y: number;
  width: number;
  height: number;
  polygon: Array<{ x: number; y: number }>;
  fitMode: "contain" | "cover";
  paddingPercent: number;
  maskOpacity: number;
  transitionDurationMs: number;
  easing: string;
  source: "detected" | "manual" | "manual_edited";
  isEnabled: boolean;
  confidence?: number;
  modelVersion?: string;
}

interface CameraInput {
  viewportWidth: number;
  viewportHeight: number;
  pageWidth: number;
  pageHeight: number;
  frame: FocusFrame;
  padding: number;
  minScale?: number;
  maxScale?: number;
}

export interface CameraTransform {
  scale: number;
  x: number;
  y: number;
}

export function calculateCamera(input: CameraInput): CameraTransform {
  const bounds = frameBounds(input.frame);
  const frameX = bounds.x * input.pageWidth;
  const frameY = bounds.y * input.pageHeight;
  const paddingFactor = 1 + input.frame.paddingPercent / 100;
  const frameWidth = Math.max(1, bounds.width * input.pageWidth * paddingFactor);
  const frameHeight = Math.max(1, bounds.height * input.pageHeight * paddingFactor);
  const availableWidth = Math.max(1, input.viewportWidth - input.padding * 2);
  const availableHeight = Math.max(1, input.viewportHeight - input.padding * 2);
  const scaleX = availableWidth / frameWidth;
  const scaleY = availableHeight / frameHeight;
  const calculatedScale = input.frame.fitMode === "cover" ? Math.max(scaleX, scaleY) : Math.min(scaleX, scaleY);
  const scale = Math.max(input.minScale ?? 0.05, Math.min(input.maxScale ?? 3, calculatedScale));
  const frameCenterX = frameX + frameWidth / 2;
  const frameCenterY = frameY + frameHeight / 2;

  return {
    scale,
    x: input.viewportWidth / 2 - frameCenterX * scale,
    y: input.viewportHeight / 2 - frameCenterY * scale,
  };
}

export function frameBounds(frame: FocusFrame) {
  if (frame.shapeType !== "polygon" || frame.polygon.length < 3) {
    return { x: frame.x, y: frame.y, width: frame.width, height: frame.height };
  }
  const xs = frame.polygon.map((point) => point.x);
  const ys = frame.polygon.map((point) => point.y);
  const x = Math.min(...xs);
  const y = Math.min(...ys);
  return { x, y, width: Math.max(...xs) - x, height: Math.max(...ys) - y };
}

export function defaultFrame(order: number, overrides: Partial<FocusFrame> = {}): FocusFrame {
  return {
    name: `Frame ${order}`,
    order,
    shapeType: "rectangle",
    frameType: "panel",
    x: 0.1,
    y: 0.1,
    width: 0.8,
    height: 0.3,
    polygon: [],
    fitMode: "contain",
    paddingPercent: 4,
    maskOpacity: 0.7,
    transitionDurationMs: 350,
    easing: "cubic-bezier(.22,.61,.36,1)",
    source: "manual",
    isEnabled: true,
    confidence: undefined,
    modelVersion: undefined,
    ...overrides,
  };
}
