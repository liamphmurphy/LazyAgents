import type { Schemas } from "@/lib/api/client";
import { SmallPill, type PillTone } from "./labels";

export const RUN_STATE: Record<string, { tone: PillTone; label: string }> = {
  queued: { tone: "neutral", label: "Queued" },
  starting: { tone: "neutral", label: "Starting" },
  running: { tone: "done", label: "Running" },
  waiting: { tone: "human", label: "Waiting" },
  waiting_for_quota: { tone: "neutral", label: "Waiting for quota" },
  paused: { tone: "human", label: "Paused" },
  blocked: { tone: "blocked", label: "Blocked" },
  completed: { tone: "review", label: "Completed" },
  failed: { tone: "blocked", label: "Failed" },
  cancelled: { tone: "neutral", label: "Cancelled" },
};
export const WAIT_LABEL: Record<string, string> = { approval: "approval", human_reply: "reply", pause: "paused", delegation: "sub agents", mockup_review: "mockup review" };
export const RUN_ENDED = ["completed", "failed", "cancelled"];

/** State as text plus tone, with the wait reason for waiting runs. */
export function StatusPill({ run }: { run: Pick<Schemas["Run"], "state" | "wait_reason"> }) {
  const s = RUN_STATE[run.state] ?? { tone: "neutral" as const, label: run.state };
  return (
    <SmallPill tone={s.tone}>
      {s.label}
      {run.state === "waiting" && run.wait_reason ? ` · ${WAIT_LABEL[run.wait_reason]}` : ""}
    </SmallPill>
  );
}
