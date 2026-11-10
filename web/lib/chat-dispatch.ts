/** A ```dispatch fence: the persona offering to start a ticket's run. */
export const isDispatchFence = (info: string | undefined) => (info ?? "").trim().split(/\s+/)[0]?.toLowerCase() === "dispatch";

/** The ticket key a ```dispatch fence names, or null while it's incomplete or isn't one. */
export function parseDispatch(code: string): string | null {
  const key = (code.trim().split("\n")[0] ?? "").trim().toUpperCase();
  return /^[A-Z][A-Z0-9]{1,9}-\d+$/.test(key) ? key : null;
}

/** Run states after which nothing more happens. */
export const RUN_ENDED_STATES = ["completed", "failed", "cancelled"];

export const runEnded = (state: string) => RUN_ENDED_STATES.includes(state);

/** "Step 2 of 4 · Fix Round()": where the agent's plan is, or "" before it posts one. */
export function planLine(run: { steps_total: number; steps_done: number; current_step?: string }): string {
  if (run.steps_total === 0) return "";
  if (!run.current_step) return `${run.steps_done} of ${run.steps_total} steps done`;
  return `Step ${Math.min(run.steps_done + 1, run.steps_total)} of ${run.steps_total} · ${run.current_step}`;
}
