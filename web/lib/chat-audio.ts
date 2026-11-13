import type { components } from "./api/schema";

export type ChatAudio = components["schemas"]["ChatAudio"];
export type AudioStage = components["schemas"]["ChatAudioStage"];

/** The pipeline the card shows, in order. */
export const AUDIO_STAGES: { id: AudioStage; label: string; failed: string; retry: string }[] = [
  { id: "compose", label: "Compose", failed: "Composing failed", retry: "Retry composing" },
  { id: "render", label: "Render", failed: "Rendering failed", retry: "Retry render" },
  { id: "check", label: "Mix check", failed: "Checking the mix failed", retry: "Retry mix check" },
  { id: "save", label: "Save to Library", failed: "Saving failed", retry: "Retry save" },
];

export type AudioStageState = "done" | "active" | "failed" | "pending";

/** Each stage's state: done before the job's stage, its stage active (or failed), the rest pending. A finished file is done throughout. */
export function audioStageStates(a: Pick<ChatAudio, "stage" | "state">): AudioStageState[] {
  const at = AUDIO_STAGES.findIndex((s) => s.id === a.stage);
  return AUDIO_STAGES.map((_, i) => {
    if (a.state === "done" || i < at) return "done";
    if (i > at) return "pending";
    return a.state === "failed" ? "failed" : "active";
  });
}

/** What Retry asks the persona to do for failed audio. */
export function audioRetryRequest(a: Pick<ChatAudio, "title" | "stage">): string {
  const step = AUDIO_STAGES.find((s) => s.id === a.stage)?.label.toLowerCase() ?? a.stage;
  return `Retry the audio "${a.title}" from the ${step} step.`;
}

/** The bars a waveform draws: the server's peaks, or a flat line for a format it can't decode. */
export const WAVE_BARS = 64;
export function waveBars(peaks: readonly number[] | undefined, count = WAVE_BARS): number[] {
  if (!peaks?.length) return Array.from({ length: count }, () => 0.08);
  return Array.from({ length: count }, (_, i) => {
    // Resample: the server sends 64 buckets, a small tile draws fewer.
    const from = Math.floor((i * peaks.length) / count);
    const to = Math.max(from + 1, Math.floor(((i + 1) * peaks.length) / count));
    let top = 0;
    for (let j = from; j < to; j++) top = Math.max(top, peaks[j]);
    return Math.max(0.08, top / 255);
  });
}

/** 1 for 1×, "1.5×" for the speed button. */
export function speedLabel(rate: number): string {
  return `${rate}×`;
}

export const SPEEDS = [0.75, 1, 1.25, 1.5, 2] as const;

/** The next speed after rate, wrapping around. */
export function nextSpeed(rate: number): number {
  const i = SPEEDS.findIndex((s) => s === rate);
  return SPEEDS[(i + 1) % SPEEDS.length];
}

/** A play position moved by seconds, kept inside the clip. */
export function skip(at: number, by: number, length: number): number {
  return Math.min(Math.max(0, at + by), Math.max(0, length));
}

/** "WAV · 44.1 kHz" style labels aren't known without decoding; the type names the format. */
export function audioFormat(contentType: string, filename: string): string {
  const ext = filename.split(".").pop()?.toUpperCase();
  if (ext && ext.length <= 4) return ext;
  return contentType.replace(/^audio\//, "").toUpperCase();
}
