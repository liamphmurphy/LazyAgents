/** A ```dispatch fence: the persona offering to start a ticket's run. */
export const isDispatchFence = (info: string | undefined) => (info ?? "").trim().split(/\s+/)[0]?.toLowerCase() === "dispatch";

/** The most tickets one ```dispatch card offers; more would be a planning job, not a card. */
export const MAX_DISPATCH_KEYS = 20;

/**
 * The ticket keys a ```dispatch fence names, one per line, in order and once each.
 * Lines that aren't a key (a half-streamed "PAY-") are skipped; [] when there are none.
 */
export function parseDispatchKeys(code: string): string[] {
  const keys: string[] = [];
  for (const line of code.split("\n")) {
    const key = line.trim().toUpperCase();
    if (/^[A-Z][A-Z0-9]{1,9}-\d+$/.test(key) && !keys.includes(key)) keys.push(key);
  }
  return keys.slice(0, MAX_DISPATCH_KEYS);
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
