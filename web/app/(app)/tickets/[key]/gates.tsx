"use client";

import Link from "next/link";
import type { Ticket, TicketStatus } from "@/lib/api/client";
import { ago } from "@/lib/format";
import { cx } from "@/lib/cx";
import { Icon, type IconName } from "@/components/ui/icon";
import { STATUS_NAME } from "@/components/domain/meta";

/** The stage a ticket moves to next; closed tickets have none. */
const NEXT_STAGE: Partial<Record<TicketStatus, TicketStatus>> = {
  backlog: "todo",
  todo: "in_progress",
  in_progress: "in_review",
  in_review: "done",
};

export type Gate = {
  id: "dependency" | "mockup" | "approval" | "pr";
  title: string;
  detail: string;
  icon: IconName;
  action: string;
  href: string;
  external?: boolean;
};

/** Unmet gates between the ticket and its next stage, in the order a person usually clears them. */
export function unmetGates(t: Ticket): Gate[] {
  const gates: Gate[] = [];
  const blockers = t.blocked_by.filter((d) => d.status !== "done");
  if (blockers.length) {
    gates.push({
      id: "dependency",
      title: blockers.length === 1 ? "Unmet dependency" : `${blockers.length} unmet dependencies`,
      detail: blockers.map((d) => `${d.key} is ${STATUS_NAME[d.status]}`).join(" · "),
      icon: "link",
      action: "View blockers",
      href: "#blocked-by",
    });
  }
  const g = t.mockup_gate;
  if (g.required && !g.approved) {
    const m = g.mockups.find((x) => x.review_state !== "approved");
    gates.push({
      id: "mockup",
      title: "Mockup not approved",
      detail: m ? `${m.title} · ${m.version_label} · ${m.review_state === "pending" ? "Pending review" : "Changes requested"}` : "No mockup yet",
      icon: "artifacts",
      action: m ? "Review mockup" : "Add a mockup",
      href: m ? `/artifacts/${m.id}/review?ticket=${encodeURIComponent(t.key)}` : "#mockups",
    });
  }
  const pending = t.approvals.filter((a) => a.state === "pending");
  if (pending.length) {
    const a = pending[0];
    gates.push({
      id: "approval",
      title: pending.length === 1 ? "Approval pending" : `${pending.length} approvals pending`,
      detail: `${a.scope} · requested by @${a.agent.handle} ${ago(a.created_at)}`,
      icon: "shield",
      action: "Review approval",
      href: `#approval-${a.id}`,
    });
  }
  if (t.pr && (t.pr.state === "open" || t.pr.state === "draft")) {
    const diff = t.pr.additions != null ? ` · +${t.pr.additions} −${t.pr.deletions ?? 0}` : "";
    gates.push({
      id: "pr",
      title: t.pr.state === "draft" ? "Draft PR" : "PR in review",
      detail: `#${t.pr.number} · ${t.pr.state === "draft" ? "Draft" : "Open"}${diff}`,
      icon: "pr",
      action: "Open PR",
      href: t.pr.url ?? "#code",
      external: !!t.pr.url,
    });
  }
  return gates;
}

/**
 * What stands between the ticket and its next stage (approved mockup). Lists
 * only unmet gates, each linking to where a person resolves it.
 */
export function GateStrip({ t }: { t: Ticket }) {
  const next = NEXT_STAGE[t.status];
  if (!next) return null;
  const gates = unmetGates(t);
  if (gates.length === 0) {
    return (
      <p className="flex items-center gap-2 rounded-xl border border-border-subtle bg-surface px-4 py-2.5 text-13 text-pill-done-fg">
        <Icon name="check" size={14} /> Ready for {STATUS_NAME[next]}
      </p>
    );
  }
  return (
    <section aria-labelledby="gates-title" className="flex flex-col gap-2.5 rounded-xl border border-border bg-surface p-3.5">
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <h2 id="gates-title" className="text-13 font-semibold">
          Before {STATUS_NAME[next]}
        </h2>
        <span className="text-12 text-fg-tertiary">What stands between this ticket and the next stage</span>
        <span className="ml-auto text-12 font-medium text-pill-human-fg">
          {gates.length} {gates.length === 1 ? "gate" : "gates"} open
        </span>
      </div>
      <ul className="grid grid-cols-[repeat(auto-fill,minmax(200px,1fr))] gap-2">
        {gates.map((g) => (
          <li key={g.id} className={cx("flex flex-col gap-1 rounded-lg border px-3 py-2.5", g.id === "pr" ? "border-border-subtle" : "border-pill-human-bg bg-pill-human-bg/30")}>
            <span className="flex items-center gap-1.5 text-13 font-medium">
              <Icon name={g.icon} size={14} className="text-fg-tertiary" />
              {g.title}
            </span>
            <span className="truncate text-12 text-fg-secondary" title={g.detail}>
              {g.detail}
            </span>
            {g.external ? (
              <a href={g.href} target="_blank" rel="noreferrer noopener" className="mt-auto text-12 text-accent hover:underline">
                {g.action} ↗
              </a>
            ) : g.href.startsWith("#") ? (
              <a href={g.href} className="mt-auto text-12 text-accent hover:underline">
                {g.action} →
              </a>
            ) : (
              <Link href={g.href} className="mt-auto text-12 text-accent hover:underline">
                {g.action} →
              </Link>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}
