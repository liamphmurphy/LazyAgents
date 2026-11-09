// Guided Learning view logic. Kept free of React so `npm run test:unit` covers it.

export type BuildState = "scoping" | "outline_ready" | "researching" | "authoring" | "verifying" | "ready" | "stopped" | "failed";

/** The steps of a build as the stepper shows them; each maps to the states it covers. */
export const BUILD_STEPS: { label: string; state: BuildState }[] = [
  { label: "Scoping", state: "scoping" },
  { label: "Outline", state: "outline_ready" },
  { label: "Research", state: "researching" },
  { label: "Authoring", state: "authoring" },
  { label: "Review", state: "verifying" },
  { label: "Ready", state: "ready" },
];

export const buildStepIndex = (state: string) => BUILD_STEPS.findIndex((s) => s.state === state);

const BUILDING = new Set<string>(["scoping", "researching", "authoring", "verifying"]);
/** Vesper is working: nothing the person does will move it on. */
export const courseBuilding = (state: string) => BUILDING.has(state);

export const COURSE_STATE_LABELS: Record<BuildState, string> = {
  scoping: "Drafting outline",
  outline_ready: "Outline ready",
  researching: "Researching",
  authoring: "Writing",
  verifying: "Reviewing",
  ready: "Ready",
  stopped: "Stopped",
  failed: "Failed",
};

export type OutlineUnitOrder = { title: string; lessons: string[] };

/**
 * Moves a lesson one place through the module's order. At the edge of a unit
 * it crosses into the neighbouring one. Returns the same array if it can't move.
 */
export function moveLesson(units: OutlineUnitOrder[], key: string, dir: -1 | 1): OutlineUnitOrder[] {
  const u = units.findIndex((x) => x.lessons.includes(key));
  if (u < 0) return units;
  const i = units[u].lessons.indexOf(key);
  const next = units.map((x) => ({ ...x, lessons: [...x.lessons] }));
  const j = i + dir;
  if (j >= 0 && j < next[u].lessons.length) {
    [next[u].lessons[i], next[u].lessons[j]] = [next[u].lessons[j], next[u].lessons[i]];
    return next;
  }
  const target = u + dir;
  if (target < 0 || target >= next.length) return units;
  next[u].lessons.splice(i, 1);
  if (dir < 0) next[target].lessons.push(key);
  else next[target].lessons.unshift(key);
  return next;
}

/** Runs still to go once the outline is approved: research, write and review per unit with something to teach. */
export function estimatedRuns(units: { lessons: { known?: boolean }[] }[]): number {
  return units.filter((u) => u.lessons.some((l) => !l.known)).length * 3;
}

/** The lesson to open: the one asked for, else the first with work left, else the first. */
export function pickLesson<L extends { key: string; status: string; progress: string; checks: unknown[] }>(lessons: L[], requested: string | null): L | undefined {
  const open = lessons.filter((l) => l.status !== "known");
  return open.find((l) => l.key === requested) ?? open.find((l) => l.progress === "new" && l.checks.length > 0) ?? open[0];
}

/** Above this a module is built in parts (the server's learningPartMinutes). */
export const PART_MINUTES = 480;

/** The time slider's stops: half hours up to 3h, then whole hours to 20h. */
export const TIME_STEPS: readonly number[] = [30, 60, 90, 120, 150, 180, ...Array.from({ length: 17 }, (_, i) => 240 + i * 60)];

export function timeLabel(minutes: number): string {
  if (minutes < 60) return `${minutes} min`;
  const h = minutes / 60;
  return `${h % 1 ? h.toFixed(1) : h} ${h === 1 ? "hour" : "hours"}`;
}

/** About how many parts a module of this length is planned as (the server's guidance). */
export const partsEstimate = (minutes: number) => (minutes > PART_MINUTES ? Math.min(Math.max(Math.floor((minutes + 120) / 240), 2), 6) : 1);

export function timeDetail(minutes: number): string {
  if (minutes <= 90) return "A focused pass over what matters most.";
  if (minutes <= 240) return "Room for the main ideas and some practice.";
  if (minutes <= PART_MINUTES) return "A thorough module to work through over time.";
  return `Built in about ${partsEstimate(minutes)} parts. The first is built now; each next one is outlined as you near it, from how you're doing.`;
}

export const CONFIDENCE = [
  { value: "1", label: "Guessing" },
  { value: "2", label: "Fairly" },
  { value: "3", label: "Certain" },
] as const;
