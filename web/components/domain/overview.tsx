"use client";

import Link from "next/link";
import type { Schemas } from "@/lib/api/client";
import { cx } from "@/lib/cx";
import { AgentAvatar } from "./avatars";
import { Pill, type PillTone } from "./labels";
import { StackedBar } from "./data";
import { STATUS_NAME } from "./meta";
import { StatusGlyph } from "./glyphs";

export type AgentOverview = Schemas["AgentOverview"];
export type EpicOverview = Schemas["EpicOverview"];
export type Counts = Schemas["WorkflowCounts"];
export const AGENT_STATUS: Record<
  Schemas["Agent"]["status"],
  { tone: PillTone; label: string; color: string }
> = {
  running: { tone: "done", label: "Running", color: "var(--state-done)" },
  waiting: { tone: "human", label: "Waiting", color: "var(--state-human)" },
  blocked: { tone: "blocked", label: "Blocked", color: "var(--state-blocked)" },
  idle: { tone: "neutral", label: "Idle", color: "var(--state-backlog)" },
};
export function AgentStatus({
  status,
}: {
  status: Schemas["Agent"]["status"];
}) {
  return (
    <Pill tone={AGENT_STATUS[status].tone}>{AGENT_STATUS[status].label}</Pill>
  );
}
export function HealthPill({ health }: { health: Schemas["Epic"]["health"] }) {
  const info = {
    on_track: { tone: "done" as const, label: "On track" },
    at_risk: { tone: "human" as const, label: "At risk" },
    off_track: { tone: "blocked" as const, label: "Off track" },
  }[health];
  return <Pill tone={info.tone}>{info.label}</Pill>;
}
export function WorkflowBar({ counts }: { counts: Counts }) {
  return (
    <StackedBar
      segments={(
        ["done", "in_review", "in_progress", "todo", "backlog"] as const
      ).map((status) => ({
        value: counts[status],
        label: STATUS_NAME[status],
        color: `var(--state-${status === "in_review" ? "review" : status === "in_progress" ? "progress" : status})`,
      }))}
    />
  );
}
export function AgentStack({ agents }: { agents: Schemas["AgentRef"][] }) {
  return (
    <div className="flex items-center -space-x-[5px]">
      {agents.slice(0, 6).map((a) => (
        <Link
          key={a.id}
          href={`/agents/${a.handle}`}
          title={a.name}
          className="rounded-md hover:z-10"
        >
          <AgentAvatar
            name={a.name}
            colorIndex={a.color_index}
            size="stack"
            ring="var(--surface)"
          />
        </Link>
      ))}
      {agents.length > 6 && (
        <span className="pl-3 font-mono text-xs text-fg-tertiary">
          +{agents.length - 6}
        </span>
      )}
      {agents.length === 0 && (
        <span className="text-xs text-fg-tertiary">No agents</span>
      )}
    </div>
  );
}
export function seconds(value: number | null | undefined) {
  if (value == null) return "—";
  if (value < 60) return `${Math.round(value)}s`;
  if (value < 3600) return `${Math.round(value / 60)}m`;
  if (value < 86400) return `${(value / 3600).toFixed(1)}h`;
  return `${(value / 86400).toFixed(1)}d`;
}
export function money(value: number) {
  return new Intl.NumberFormat(undefined, {
    style: "currency",
    currency: "USD",
    maximumFractionDigits: 2,
  }).format(value);
}
export function merged(stats: Schemas["PerformanceStats"]) {
  return stats.closed_prs
    ? `${Math.round((stats.merged_prs / stats.closed_prs) * 100)}%`
    : "—";
}
export function WorkRow({ ticket }: { ticket: Schemas["WorkTicket"] }) {
  return (
    <Link
      href={`/tickets/${ticket.key}`}
      className="flex min-h-11 items-center gap-3 border-t border-border-row px-4 py-2 hover:bg-row-hover"
    >
      <StatusGlyph state={ticket.status} />
      <span className="shrink-0 font-mono text-xs text-fg-tertiary">
        {ticket.key}
      </span>
      <span className="min-w-0 truncate text-[13px] font-medium">
        {ticket.title}
      </span>
    </Link>
  );
}
export function Assignment({
  work,
  compact = false,
}: {
  work: Schemas["AgentWork"];
  compact?: boolean;
}) {
  const done = work.steps_done,
    total = work.steps_total;
  return (
    <Link
      href={`/runs/${work.run_id}`}
      className={cx(
        "flex flex-col gap-2 rounded-lg border border-border-subtle bg-surface-sunken p-3 hover:border-border-hover",
        compact && "p-2.5",
      )}
    >
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
        <span className="font-mono text-xs text-fg-secondary">
          {work.ticket.key}
        </span>
        <span className="ml-auto text-xs text-fg-tertiary">
          {work.state.replaceAll("_", " ")}
          {total > 0 && ` · step ${work.step_position || done}/${total}`}
        </span>
      </div>
      <span className="line-clamp-2 text-[13px] text-fg-2">
        {work.step || work.ticket.title}
      </span>
      {total > 0 && (
        <span
          role="progressbar"
          aria-label={`${work.ticket.key} plan steps`}
          aria-valuenow={done}
          aria-valuemax={total}
          aria-valuemin={0}
          className="h-[3px] rounded-sm bg-track"
        >
          <span
            className="block h-full rounded-sm bg-accent"
            style={{ width: `${Math.round((done / total) * 100)}%` }}
          />
        </span>
      )}
      <span className="truncate font-mono text-[10.5px] text-fg-tertiary">
        {work.harness} · {work.model}
      </span>
    </Link>
  );
}
