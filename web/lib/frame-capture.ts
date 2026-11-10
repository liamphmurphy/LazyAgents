// A screenshot of the playground's Preview frame. The frame is another
// origin, so the page can't draw it; the browser's own tab capture can, with
// the person's say-so each time (getDisplayMedia). Chrome's Region Capture
// crops the stream to the frame itself; elsewhere the whole tab is captured
// and cropped here, by the frame's place in the viewport.

type Rect = { x: number; y: number; width: number; height: number };

/**
 * Where the frame is in a captured video frame: its viewport rectangle scaled
 * to the video's pixels. Null when the capture doesn't look like this tab's
 * viewport (the person shared a window or another tab), when the crop would
 * be wrong.
 */
export function cropFor(frame: Rect, viewport: { width: number; height: number }, video: { width: number; height: number }): Rect | null {
  if (!video.width || !video.height || !viewport.width || !viewport.height) return null;
  const sx = video.width / viewport.width;
  const sy = video.height / viewport.height;
  if (Math.abs(sx - sy) / sx > 0.05) return null; // another aspect ratio: not this viewport
  const x = Math.max(0, Math.round(frame.x * sx));
  const y = Math.max(0, Math.round(frame.y * sy));
  const width = Math.min(video.width - x, Math.round(frame.width * sx));
  const height = Math.min(video.height - y, Math.round(frame.height * sy));
  return width > 0 && height > 0 ? { x, y, width, height } : null;
}

export class CaptureError extends Error {}

type CropTargetStatic = { fromElement(el: Element): Promise<unknown> };
type CroppableTrack = MediaStreamTrack & { cropTo?: (target: unknown) => Promise<void> };

/** A PNG of el as the person sees it, or a CaptureError saying why not. */
export async function captureElement(el: Element): Promise<Blob> {
  if (!navigator.mediaDevices?.getDisplayMedia) throw new CaptureError("this browser can't capture the screen");
  let stream: MediaStream;
  try {
    stream = await navigator.mediaDevices.getDisplayMedia({
      video: { displaySurface: "browser" },
      audio: false,
      // Chrome: offer this tab first, and include it at all.
      ...({ preferCurrentTab: true, selfBrowserSurface: "include" } as object),
    });
  } catch (e) {
    throw new CaptureError(e instanceof DOMException && e.name === "NotAllowedError" ? "sharing was cancelled" : "the screen couldn't be captured");
  }
  const [track] = stream.getVideoTracks() as CroppableTrack[];
  try {
    const CropTarget = (globalThis as { CropTarget?: CropTargetStatic }).CropTarget;
    let cropped = false;
    if (CropTarget && track.cropTo) {
      try {
        await track.cropTo(await CropTarget.fromElement(el));
        cropped = true;
      } catch {
        // Another tab or a window was shared: crop by hand below, if it fits.
      }
    }
    const video = document.createElement("video");
    video.muted = true;
    video.srcObject = stream;
    await video.play();
    // The first frame can still show the share prompt fading out.
    await new Promise((r) => setTimeout(r, 250));
    const size = { width: video.videoWidth, height: video.videoHeight };
    const r = el.getBoundingClientRect();
    const crop = cropped ? { x: 0, y: 0, ...size } : cropFor({ x: r.x, y: r.y, width: r.width, height: r.height }, { width: window.innerWidth, height: window.innerHeight }, size);
    const area = crop ?? { x: 0, y: 0, ...size };
    const canvas = document.createElement("canvas");
    canvas.width = area.width;
    canvas.height = area.height;
    canvas.getContext("2d")?.drawImage(video, area.x, area.y, area.width, area.height, 0, 0, area.width, area.height);
    video.srcObject = null;
    const blob = await new Promise<Blob | null>((res) => canvas.toBlob(res, "image/png"));
    if (!blob) throw new CaptureError("the screenshot couldn't be encoded");
    return blob;
  } finally {
    stream.getTracks().forEach((t) => t.stop());
  }
}
