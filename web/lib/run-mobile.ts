// Pure view logic for the compact Runs list and run detail (DESIGN.md §3
// Responsive, frames "7 Runs" and "8 Run detail"). Unit-tested in
// run-mobile.test.mjs; the screens are app/(app)/runs/runs-compact.tsx and
// app/(app)/runs/[id]/run-compact.tsx.

type Step = { position: number; text: string; state: "pending" | "current" | "done" | "needs_human" | "failed" };
type PlanRun = { plan: Step[] };
type EndRun = { state: string; ended_at?: string | null };

export type StepProgress = {
  done: number;
  total: number;
  /** The current step (or the first one needing a person or failed), if any. */
  current?: Step;
  /** One per step, in plan order. */
  steps: Step[];
};

/** How far a run's plan has got: done steps, the step in hand, every segment. */
export function stepProgress(run: PlanRun): StepProgress {
  const steps = [...run.plan].sort((a, b) => a.position - b.position);
  const done = steps.filter((s) => s.state === "done").length;
  const current =
    steps.find((s) => s.state === "current") ??
    steps.find((s) => s.state === "needs_human" || s.state === "failed");
  return { done, total: steps.length, current, steps };
}

const ENDED = ["completed", "failed", "cancelled"];

/** Whether a run ended on the same local calendar day as `now`. */
export function endedToday(run: EndRun, now: Date): boolean {
  if (!ENDED.includes(run.state) || !run.ended_at) return false;
  const end = new Date(run.ended_at);
  return end.getFullYear() === now.getFullYear() && end.getMonth() === now.getMonth() && end.getDate() === now.getDate();
}

/** Runs that ended today, latest first. */
export function finishedToday<R extends EndRun>(runs: R[], now: Date): R[] {
  return runs
    .filter((r) => endedToday(r, now))
    .sort((a, b) => Date.parse(b.ended_at!) - Date.parse(a.ended_at!));
}

/** The finished-today card's status: Done, Failed or Cancelled. */
export function finishedLabel(state: string): { label: string; tone: "done" | "blocked" | "neutral" } {
  if (state === "completed") return { label: "Done", tone: "done" };
  if (state === "failed") return { label: "Failed", tone: "blocked" };
  return { label: "Cancelled", tone: "neutral" };
}

/** A live card's pill: amber "Needs you" when a person unblocks it, else the state. */
export function livePill(run: { state: string; wait_reason?: string | null }): { label: string; tone: "done" | "human" | "neutral" } {
  if (run.state === "paused" || (run.state === "waiting" && ["approval", "human_reply", "mockup_review"].includes(run.wait_reason ?? ""))) {
    return { label: "Needs you", tone: "human" };
  }
  if (run.state === "running") return { label: "Running", tone: "done" };
  if (run.state === "waiting") return { label: "Waiting", tone: "neutral" };
  if (run.state === "waiting_for_quota") return { label: "Waiting for quota", tone: "neutral" };
  return { label: run.state === "queued" ? "Queued" : run.state === "starting" ? "Starting" : run.state, tone: "neutral" };
}
