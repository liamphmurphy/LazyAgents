"use client";

import Link from "next/link";
import type { Run } from "@/lib/api/client";
import { useCosts } from "@/lib/api/hooks";
import { spendLabel, usd } from "@/lib/costs";
import { Icon } from "@/components/ui/icon";

/** A header pill with spend; links to the Costs page. */
export function CostChip({ value, title, href = "/reports?tab=costs" }: { value: string; title: string; href?: string }) {
  return (
    <Link
      href={href}
      title={title}
      aria-label={`${title}: ${value}`}
      data-testid="cost-chip"
      className="flex flex-none items-center gap-1 whitespace-nowrap rounded-full border border-border-strong py-0.5 pl-1.5 pr-2.5 font-mono text-[11.5px] text-fg-secondary hover:bg-hover hover:text-fg"
    >
      <Icon name="dollar" size={12} className="text-fg-tertiary" />
      {value}
    </Link>
  );
}

/**
 * A run's own counter, the figure every Costs aggregate sums, labelled with the
 * same unpriced rule as GET /costs: unknown or partial, or tokens without a source.
 */
export function RunCostChip({ run }: { run: Pick<Run, "cost_usd" | "cost_source" | "input_tokens" | "output_tokens"> }) {
  const src = run.cost_source;
  const unpriced = src === "unknown" || src === "partial" || (!src && run.input_tokens + run.output_tokens > 0);
  const micros = Math.round(run.cost_usd * 1e6);
  return <CostChip value={unpriced ? spendLabel(micros, 1) : usd(micros)} title={src === "computed" ? "This run's spend, computed from token prices" : "This run's spend"} />;
}

/** All-time spend on a ticket, sub agent runs included, from the same aggregate as the Costs page. */
export function TicketCostChip({ ticketKey, project }: { ticketKey: string; project?: string }) {
  const q = useCosts({ ticket: ticketKey });
  if (!q.data || q.data.total.runs === 0) return null;
  const t = q.data.total;
  return (
    <CostChip
      value={spendLabel(t.cost_micros, q.data.unpriced_runs)}
      title={`Spend on ${ticketKey} across ${t.runs} ${t.runs === 1 ? "run" : "runs"}, sub agents included`}
      href={project ? `/reports?tab=costs&project=${encodeURIComponent(project)}` : "/reports?tab=costs"}
    />
  );
}

/** All-time spend on a chat, every run segment included. */
export function ChatCostChip({ chatId, project }: { chatId: string; project?: string }) {
  const q = useCosts({ chat: chatId });
  if (!q.data || q.data.total.runs === 0) return null;
  return (
    <CostChip
      value={spendLabel(q.data.total.cost_micros, q.data.unpriced_runs)}
      title="Spend on this chat"
      href={project ? `/reports?tab=costs&project=${encodeURIComponent(project)}` : "/reports?tab=costs"}
    />
  );
}
