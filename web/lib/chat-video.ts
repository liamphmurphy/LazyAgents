import type { components } from "./api/schema";

export type ChatVideo = components["schemas"]["ChatVideo"];
export type VideoStage = components["schemas"]["ChatVideoStage"];

/** The pipeline the card shows, in order. */
export const VIDEO_STAGES: { id: VideoStage; label: string; failed: string; retry: string }[] = [
  { id: "write", label: "Write renderer", failed: "Writing the renderer failed", retry: "Retry writing" },
  { id: "stills", label: "Check stills", failed: "Checking the stills failed", retry: "Retry stills" },
  { id: "render", label: "Render frames", failed: "Rendering failed", retry: "Retry render" },
  { id: "encode", label: "Encode", failed: "Encoding failed", retry: "Retry encode" },
  { id: "save", label: "Save to Library", failed: "Saving failed", retry: "Retry save" },
];

export type StageState = "done" | "active" | "failed" | "pending";

/** Each stage's state: done before the job's stage, its stage active (or failed), the rest pending. A finished video is done throughout. */
export function stageStates(v: Pick<ChatVideo, "stage" | "state">): StageState[] {
  const at = VIDEO_STAGES.findIndex((s) => s.id === v.stage);
  return VIDEO_STAGES.map((_, i) => {
    if (v.state === "done" || i < at) return "done";
    if (i > at) return "pending";
    return v.state === "failed" ? "failed" : "active";
  });
}

/** "0:15", "1:02:03". */
export function clock(ms: number): string {
  const s = Math.max(0, Math.floor(ms / 1000));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const ss = String(s % 60).padStart(2, "0");
  return h ? `${h}:${String(m).padStart(2, "0")}:${ss}` : `${m}:${ss}`;
}

/** "1920×1080 · 30 fps · 15 s" as planned. */
export function plannedMeta(v: Pick<ChatVideo, "width" | "height" | "fps" | "duration_ms">): string {
  const secs = v.duration_ms / 1000;
  return `${v.width}×${v.height} · ${v.fps} fps · ${Number.isInteger(secs) ? secs : secs.toFixed(1)} s`;
}

/**
 * Rendering progress: "Frame 212 of 450 · ~1m 40s left", from the frames done since the render
 * began. Before there's a rate, just the frame count.
 */
export function renderEta(v: Pick<ChatVideo, "frames_done" | "frames_total" | "render_started_at">, now: number): string {
  if (!v.frames_total) return "Starting the render…";
  const head = `Frame ${v.frames_done.toLocaleString()} of ${v.frames_total.toLocaleString()}`;
  const started = v.render_started_at ? Date.parse(v.render_started_at) : NaN;
  const elapsed = (now - started) / 1000;
  if (!v.frames_done || !(elapsed > 1)) return head;
  const left = Math.round(((v.frames_total - v.frames_done) * elapsed) / v.frames_done);
  if (left <= 0) return head;
  return `${head} · ~${Math.floor(left / 60)}m ${String(left % 60).padStart(2, "0")}s left`;
}

/** "rendered in 3m 08s" from when the render began to when the video finished. */
export function renderTook(v: Pick<ChatVideo, "render_started_at" | "finished_at">): string | null {
  if (!v.render_started_at || !v.finished_at) return null;
  const s = Math.round((Date.parse(v.finished_at) - Date.parse(v.render_started_at)) / 1000);
  if (!(s >= 0)) return null;
  return s < 60 ? `${s}s` : `${Math.floor(s / 60)}m ${String(s % 60).padStart(2, "0")}s`;
}

/** The renderer's file name from its workspace path. */
export function scriptName(path: string): string {
  return path.split("/").pop() || path;
}

/** The URL of one of a video's frames (cached by version). */
export function frameUrl(chatId: string, v: ChatVideo, kind: string, idx: number): string | undefined {
  const f = v.frames.find((x) => x.kind === kind && x.idx === idx);
  return f && `/api/v1/chats/${chatId}/videos/${v.id}/frames/${kind}/${idx}?v=${f.version}`;
}

/** The picture to show while rendering: the latest frame, else the furthest filmstrip cell, else the last still. */
export function previewFrame(v: ChatVideo): { kind: string; idx: number } | undefined {
  const of = (kind: string) => v.frames.filter((f) => f.kind === kind).sort((a, b) => a.idx - b.idx);
  return of("preview")[0] ?? of("strip").at(-1) ?? of("still").at(-1);
}

/** The video's poster: its poster frame, else the middle still, else the latest preview. */
export function posterFrame(v: ChatVideo): { kind: string; idx: number } | undefined {
  const of = (kind: string) => v.frames.filter((f) => f.kind === kind).sort((a, b) => a.idx - b.idx);
  const stills = of("still");
  return of("poster")[0] ?? stills[Math.floor(stills.length / 2)] ?? of("preview")[0];
}

/** What Retry asks the persona to do for a failed video. */
export function retryRequest(v: Pick<ChatVideo, "title" | "stage">): string {
  const step = VIDEO_STAGES.find((s) => s.id === v.stage)?.label.toLowerCase() ?? v.stage;
  return `Retry the video "${v.title}" from the ${step} step.`;
}
