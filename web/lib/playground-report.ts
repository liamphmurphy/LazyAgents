// "Send to chat" from a playground: what the gateway's probe reports from the
// Preview frame, and the report that goes with the person's next message.
// Kept free of React so `npm run test:unit` covers it.

import type { LogLine } from "./playground.ts";

export type FailedRequest = { method: string; url: string; status: number; id: string; error: string; t: number };
export type ConsoleError = { text: string; t: number };

/** What the probe in the Preview frame has told us since the frame last loaded. */
export type ProbeState = { path: string | null; title: string; console: ConsoleError[]; requests: FailedRequest[] };

export const emptyProbe = (): ProbeState => ({ path: null, title: "", console: [], requests: [] });

/** Kept per kind; older entries drop off. */
export const MAX_PROBE_ENTRIES = 50;

const str = (v: unknown, max = 2000) => (typeof v === "string" ? v.slice(0, max) : "");
const keep = <T>(xs: T[], x: T) => (xs.length >= MAX_PROBE_ENTRIES ? [...xs.slice(1), x] : [...xs, x]);

/**
 * The state after one message from the probe. The message comes from the
 * app's page, so nothing in it is trusted: fields are read as plain strings
 * and numbers, and anything else leaves the state as it was.
 */
export function applyProbe(s: ProbeState, data: unknown, now: number): ProbeState {
  if (!data || typeof data !== "object" || (data as { vesperProbe?: unknown }).vesperProbe !== 1) return s;
  const m = data as Record<string, unknown>;
  switch (m.type) {
    case "location":
      return { ...s, path: str(m.path) || "/", title: str(m.title, 200) };
    case "console":
      return str(m.text) ? { ...s, console: keep(s.console, { text: str(m.text), t: now }) } : s;
    case "request": {
      const status = typeof m.status === "number" && Number.isFinite(m.status) ? m.status : 0;
      return { ...s, requests: keep(s.requests, { method: str(m.method, 16) || "GET", url: str(m.url), status, id: str(m.id, 200), error: str(m.error), t: now }) };
    }
  }
  return s;
}

/** Log lines that belong to a failed request: those naming its id, else those written around it. */
export function logsFor(r: FailedRequest, lines: LogLine[], max = 20): LogLine[] {
  const byId = r.id ? lines.filter((l) => l.text.includes(r.id)) : [];
  const out = byId.length ? byId : lines.filter((l) => Math.abs(Date.parse(l.t) - r.t) <= 1500 && l.stream === "stderr");
  return out.slice(-max);
}

const fence = (s: string) => {
  const ticks = "`".repeat(Math.max(3, ...[...s.matchAll(/`+/g)].map((m) => m[0].length + 1)));
  return `${ticks}\n${s}\n${ticks}`;
};
const time = (t: number) => new Date(t).toISOString().slice(11, 19);

export type ReportInput = {
  origin: string;
  probe: ProbeState;
  logs: LogLine[];
  branch?: string | null;
  at: number;
  /** Why there's no screenshot, when there isn't one. */
  noScreenshot?: string;
};

/** The Markdown attached to the next message, written for the agent to act on. */
export function playgroundReport({ origin, probe, logs, branch, at, noScreenshot }: ReportInput): string {
  const out: string[] = ["# Playground report", ""];
  out.push(`- Page: ${probe.path === null ? "unknown (the page didn't load Vesper's probe)" : `\`${probe.path}\``}${probe.title ? ` (${probe.title})` : ""}`);
  out.push(`- Served at: ${origin}`);
  if (branch) out.push(`- Branch: \`${branch}\``);
  out.push(`- Sent: ${new Date(at).toISOString()}`);
  out.push(`- Screenshot: ${noScreenshot ? `none (${noScreenshot})` : "attached"}`);
  out.push("", `## Console errors (${probe.console.length})`, "");
  if (probe.console.length === 0) out.push("None since the page loaded.");
  for (const c of probe.console) out.push(`${time(c.t)}`, fence(c.text), "");
  out.push("", `## Failed requests (${probe.requests.length})`, "");
  if (probe.requests.length === 0) out.push("None since the page loaded.");
  for (const r of probe.requests) {
    const what = r.status ? String(r.status) : `no response${r.error ? `: ${r.error}` : ""}`;
    out.push(`### ${r.method} ${r.url} → ${what}`, "", `${time(r.t)}${r.id ? ` · request id \`${r.id}\`` : ""}`, "");
    const lines = logsFor(r, logs);
    if (lines.length) out.push(fence(lines.map((l) => `[${l.source}] ${l.text}`).join("\n")), "");
  }
  return out.join("\n").trimEnd() + "\n";
}

/** Attachment names: one report and one screenshot per send, distinct between sends. */
export function reportNames(at: number): { report: string; screenshot: string } {
  const stamp = new Date(at).toISOString().slice(0, 19).replace(/[-:]/g, "").replace("T", "-");
  return { report: `playground-${stamp}.md`, screenshot: `playground-${stamp}.png` };
}

/** What the playground panel puts in the chat's composer: files to attach, text to send. */
export type ComposerDrop = { files?: File[]; text?: string };

/**
 * Hands things from the playground panel to the chat's composer, which sit in
 * different parts of the page. A composer listens for its chat; a send with
 * no composer listening reports false so the panel can say so.
 */
const listeners = new Map<string, Set<(drop: ComposerDrop) => void>>();

export function onComposerDrop(chatId: string, fn: (drop: ComposerDrop) => void): () => void {
  const set = listeners.get(chatId) ?? new Set();
  set.add(fn);
  listeners.set(chatId, set);
  return () => {
    set.delete(fn);
    if (set.size === 0) listeners.delete(chatId);
  };
}

export function sendToComposer(chatId: string, drop: ComposerDrop): boolean {
  const set = listeners.get(chatId);
  if (!set?.size) return false;
  set.forEach((fn) => fn(drop));
  return true;
}
