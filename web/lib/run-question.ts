type Ev = { type: string; payload: unknown };
type P = Record<string, unknown>;

const WAIT_KEYWORDS = ["approval", "human_reply", "pause", "delegation"];

/**
 * The agent's latest question to a person: the `reason` of its last
 * `await_human` tool call, else the reason on a `waiting` lifecycle event
 * (ignoring bare wait-kind keywords). Null when the log has neither.
 */
export function latestQuestion(events: readonly Ev[]): string | null {
  for (let i = events.length - 1; i >= 0; i--) {
    const e = events[i];
    const p = (e.payload ?? {}) as P;
    if (e.type === "tool_call" && typeof p.tool === "string" && /(^|__|\.)await_human$/.test(p.tool)) {
      const reason = ((p.input ?? {}) as P).reason;
      if (typeof reason === "string" && reason.trim()) return reason.trim();
    }
    if (e.type === "lifecycle" && p.state === "waiting" && typeof p.reason === "string") {
      const reason = p.reason.trim();
      if (reason && !WAIT_KEYWORDS.includes(reason)) return reason;
    }
  }
  return null;
}

/** Prefix `@handle ` unless the text already mentions that handle. */
export function withMention(body: string, handle: string): string {
  const text = body.trim();
  const escaped = handle.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  if (new RegExp(`(^|[^\\w@])@${escaped}(?![\\w-])`, "i").test(text)) return text;
  return `@${handle} ${text}`;
}
