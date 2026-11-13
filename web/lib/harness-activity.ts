// Usage and activity shared by the sidebar footer and its drawer.

export type UsageWindow = { id: string; kind: "session" | "weekly" | "other"; label: string; used_percent: number; resets_at?: string };
export type HarnessActivity = {
  harness: string;
  label: string;
  running: number;
  limited_until?: string;
  plan?: string;
  usage_available: boolean;
  usage_checked_at?: string;
  usage_checking?: boolean;
  windows: UsageWindow[];
};

export const CLAUDE = "claude-code";
export const CODEX = "codex";
export const ANTIGRAVITY = "antigravity";
/** Stable footer order, including harnesses an older server has not reported yet. */
export const HARNESS_ORDER = [CLAUDE, CODEX, ANTIGRAVITY] as const;

/** Running and not out of allowance: a limited harness's runs have moved elsewhere. */
export function harnessWorking(h: HarnessActivity | undefined): boolean {
  return !!h && h.running > 0 && !h.limited_until;
}

/** The session window's used percentage for a meter: 100 while limited, null when unread. */
export function sessionPercent(h: HarnessActivity | undefined): number | null {
  if (!h) return null;
  if (h.limited_until) return 100;
  if (!h.usage_available) return null;
  const w = h.windows.find((x) => x.kind === "session") ?? h.windows[0];
  return w ? Math.round(w.used_percent) : null;
}

/** Used percentage from which a usage meter warns that the limit is near. */
export const WARN_AT = 80;

export type UsageTone = "ok" | "warn" | "limited";

/** How worried a harness's meter looks: amber near the limit, red once it's hit. */
export function usageTone(h: HarnessActivity | undefined): UsageTone {
  if (h?.limited_until) return "limited";
  const pct = sessionPercent(h);
  return pct !== null && pct >= WARN_AT ? "warn" : "ok";
}

export function weeklyWindow(h: HarnessActivity): UsageWindow | undefined {
  return h.windows.find((w) => w.kind === "weekly");
}

export function clockTime(iso: string): string {
  return new Date(iso).toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
}

/** "9:20 PM" today, otherwise "Thu 8:00 AM". */
export function resetLabel(iso: string, now: Date = new Date()): string {
  const d = new Date(iso);
  const sameDay = d.toDateString() === now.toDateString();
  return sameDay ? clockTime(iso) : `${d.toLocaleDateString([], { weekday: "short" })} ${clockTime(iso)}`;
}

/** The line under a harness's usage bar. */
export function usageSub(h: HarnessActivity, all: HarnessActivity[], now: Date = new Date()): string {
  if (h.limited_until) {
    const other = all.find((x) => x.harness !== h.harness && !x.limited_until);
    return `Limited until ${resetLabel(h.limited_until, now)}${other ? ` · falls back to ${other.label}` : ""}`;
  }
  if (h.harness === ANTIGRAVITY && !h.usage_available) return "Usage unknown · Antigravity doesn’t report an allowance";
  if (!h.usage_available) {
    if (h.usage_checking) return "Checking usage…";
    return h.usage_checked_at ? "Usage unavailable · check Usage settings" : "Usage not checked yet";
  }
  const session = h.windows.find((w) => w.kind === "session");
  const weekly = weeklyWindow(h);
  const parts = [session ? (session.resets_at ? `5h · resets ${resetLabel(session.resets_at, now)}` : "5h") : "", weekly ? `weekly ${Math.round(weekly.used_percent)}%` : ""];
  return parts.filter(Boolean).join(" · ");
}

/** "checked 40s ago", from the newest reading, or "checking…" while a check is in flight. */
export function checkedAgo(harnesses: HarnessActivity[], now: number = Date.now()): string {
  if (harnesses.some((h) => h.usage_checking)) return "checking…";
  const at = harnesses.map((h) => (h.usage_checked_at ? Date.parse(h.usage_checked_at) : 0)).reduce((a, b) => Math.max(a, b), 0);
  if (!at) return "not checked yet";
  const s = Math.max(0, Math.round((now - at) / 1000));
  if (s < 60) return `checked ${s}s ago`;
  const m = Math.round(s / 60);
  return m < 60 ? `checked ${m}m ago` : `checked ${Math.round(m / 60)}h ago`;
}

const LABELS = { [CLAUDE]: "Claude", [CODEX]: "Codex", [ANTIGRAVITY]: "Antigravity" };

/** Missing readings stay visible as unknown, rather than disappearing or becoming zero. */
export function knownHarnesses(readings: HarnessActivity[]): HarnessActivity[] {
  return HARNESS_ORDER.map((harness) => readings.find((h) => h.harness === harness) ?? {
    harness, label: LABELS[harness], running: 0, usage_available: false, windows: [],
  });
}
