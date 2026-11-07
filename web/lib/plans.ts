// Pure logic for the Plans screen (LAZYAGENTS-52): index row status and
// counts, and the sources a live planner has read, from its run events.
// Kept free of React so `npm run test:unit` covers it.

import { activityOf, bareTool, toolCalls, type Activity, type RunEventLike } from "./chat-ui.ts";

export type PlanLike = {
  state: "planning" | "review" | "accepted" | "dismissed";
  run_state: string;
  goal: string;
  proposal_count: number;
  ticket_keys: string[];
};

export type PlanTone = "human" | "blocked" | "review" | "done" | "neutral";

const STOPPED = ["failed", "cancelled", "blocked"];

/** The planner is still queued or working: no draft yet and the run hasn't stopped. */
export function planLive(p: Pick<PlanLike, "state" | "run_state">): boolean {
  return p.state === "planning" && !STOPPED.includes(p.run_state) && p.run_state !== "completed";
}

/** Status pill for a plan: the review state once a draft exists, else how the run is doing. */
export function planStatus(p: Pick<PlanLike, "state" | "run_state">): { label: string; tone: PlanTone } {
  switch (p.state) {
    case "review":
      return { label: "Review", tone: "human" };
    case "accepted":
      return { label: "Accepted", tone: "done" };
    case "dismissed":
      return { label: "Dismissed", tone: "neutral" };
  }
  if (p.run_state === "cancelled") return { label: "Cancelled", tone: "neutral" };
  if (STOPPED.includes(p.run_state) || p.run_state === "completed") return { label: "Failed", tone: "blocked" };
  if (p.run_state === "queued") return { label: "Queued", tone: "neutral" };
  return { label: "Planning", tone: "review" };
}

/** Right-hand count on an index row: "reading…" while live, "no proposals", else "accepted/total accepted". */
export function planCount(p: PlanLike): string {
  if (planLive(p)) return "reading…";
  if (p.proposal_count === 0) return "no proposals";
  const accepted = p.state === "accepted" ? p.ticket_keys.length : 0;
  // Reviewers can add proposals before accepting; never show more accepted than total.
  return `${accepted}/${Math.max(accepted, p.proposal_count)} accepted`;
}

/** The goal's first non-empty line, for the index. */
export function firstLine(goal: string): string {
  return goal.split("\n").map((l) => l.trim()).find(Boolean) ?? "";
}

/** Tools that read a source (as opposed to searching or acting). */
const READS = new Set(["read_page", "read_artifact", "get_ticket", "get_backlinks"]);
const FILE_READS = new Set(["Read", "read", "read_file"]);

export type ReadSource = { key: string; activity: Activity; done: boolean };

const basename = (p: string) => p.split("/").filter(Boolean).pop() ?? p;

/**
 * What the planner has read so far, oldest first, one row per source: MCP reads
 * (pages, artifacts, tickets) and the context files the run was given.
 */
export function readSources(events: RunEventLike[] | undefined): ReadSource[] {
  const out: ReadSource[] = [];
  const at = new Map<string, number>();
  for (const call of toolCalls(events)) {
    const tool = bareTool(call.tool);
    let activity: Activity;
    if (READS.has(tool)) {
      activity = activityOf(call);
    } else if (FILE_READS.has(tool)) {
      const input = (call.input && typeof call.input === "object" ? call.input : {}) as Record<string, unknown>;
      const path = typeof input.file_path === "string" ? input.file_path : typeof input.path === "string" ? input.path : "";
      if (!path.includes(".lazyagents/")) continue;
      activity = { verb: "Reading", text: basename(path) };
    } else continue;
    const key = [tool === "get_ticket" ? "ticket" : "doc", activity.ref ?? activity.text ?? activity.verb].join(":");
    const seen = at.get(key);
    if (seen !== undefined) {
      out[seen].done ||= call.done;
      continue;
    }
    at.set(key, out.length);
    out.push({ key, activity, done: call.done });
  }
  return out;
}
