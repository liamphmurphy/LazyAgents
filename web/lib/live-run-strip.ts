// Pure helpers for the live-run strip: the compact shell's one-line stand-in
// for the activity orb, above the chat composer. No React or fetch here so
// node --test can cover them.

type Step = { position: number; state: "pending" | "current" | "done" | "needs_human" | "failed" };
export type StripRun = {
  id: string;
  ticket_key: string;
  work_title?: string;
  state: string;
  started_at: string;
  agent: { name: string };
  plan: Step[];
};

export type Strip<R extends StripRun> = {
  run: R;
  /** The step in hand, from 1, or null before a plan is posted. */
  step: number | null;
  total: number;
  /** Other live runs, shown as "+N more". */
  more: number;
  /** Whether the run waits on a person rather than working. */
  waiting: boolean;
};

const WAITING = new Set(["waiting", "blocked", "paused", "waiting_for_quota"]);

/** "step N of M": the current (or first unfinished) step; every step done reads as the last. */
export function stepOf(plan: Step[]): { step: number | null; total: number } {
  const total = plan.length;
  if (total === 0) return { step: null, total };
  const steps = [...plan].sort((a, b) => a.position - b.position);
  const i = steps.findIndex((s) => s.state === "current");
  const at = i >= 0 ? i : steps.findIndex((s) => s.state !== "done");
  return { step: (at >= 0 ? at : total - 1) + 1, total };
}

/** The newest live run and how many others there are; null with none. */
export function liveStrip<R extends StripRun>(runs: R[]): Strip<R> | null {
  if (runs.length === 0) return null;
  const run = runs.reduce((a, b) => (Date.parse(b.started_at) > Date.parse(a.started_at) ? b : a));
  return { run, ...stepOf(run.plan), more: runs.length - 1, waiting: WAITING.has(run.state) };
}

/** What the strip names the work by: the ticket key, else the plan or chat title. */
export function stripWork(run: StripRun): string {
  return run.ticket_key || run.work_title || "Run";
}

/** The strip's accessible name, e.g. "Keel on VESPER-11, step 2 of 4". */
export function stripLabel(s: Strip<StripRun>): string {
  const step = s.step === null ? "" : `, step ${s.step} of ${s.total}`;
  return `${s.run.agent.name} on ${stripWork(s.run)}${step}${s.waiting ? ", waiting" : ""}`;
}
