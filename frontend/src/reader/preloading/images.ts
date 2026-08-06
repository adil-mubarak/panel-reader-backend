interface CachedImage {
  image: HTMLImageElement;
  promise: Promise<HTMLImageElement>;
  used: number;
}

const MAX_IMAGES = 5;
const decodedImages = new Map<string, CachedImage>();
let clock = 0;

export function preloadImage(url: string): Promise<HTMLImageElement> {
  const existing = decodedImages.get(url);
  if (existing) {
    existing.used = ++clock;
    return existing.promise;
  }

  const image = new Image();
  image.decoding = "async";
  const promise = new Promise<HTMLImageElement>((resolve, reject) => {
    image.onload = () => {
      const decoded = typeof image.decode === "function" ? image.decode() : Promise.resolve();
      void decoded.catch(() => undefined).then(() => resolve(image));
    };
    image.onerror = () => reject(new Error(`Could not load image: ${url}`));
  });
  decodedImages.set(url, { image, promise, used: ++clock });
  image.src = url;
  promise.catch(() => decodedImages.delete(url));
  trimCache(new Set([url]));
  return promise;
}

export function preloadWindow(urls: string[], currentIndex: number): void {
  const retained = new Set(urls.slice(Math.max(0, currentIndex - 2), currentIndex + 3));
  retained.forEach((url) => void preloadImage(url).catch(() => undefined));
  trimCache(retained);
}

export function clearImageCache(): void {
  decodedImages.forEach(({ image }) => {
    image.onload = null;
    image.onerror = null;
    image.src = "";
  });
  decodedImages.clear();
}

function trimCache(retained: Set<string>): void {
  const candidates = [...decodedImages.entries()]
    .filter(([url]) => !retained.has(url))
    .sort((a, b) => a[1].used - b[1].used);
  while (decodedImages.size > MAX_IMAGES && candidates.length) {
    const [url, entry] = candidates.shift()!;
    entry.image.onload = null;
    entry.image.onerror = null;
    entry.image.src = "";
    decodedImages.delete(url);
  }
}
