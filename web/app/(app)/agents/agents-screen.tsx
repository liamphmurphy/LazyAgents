"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import {
  useBootstrap,
  useCreateAgent,
  useAgentOverviews,
} from "@/lib/api/hooks";
import { Button } from "@/components/ui/button";
import {
  Banner,
  EmptyState,
  Field,
  Skeleton,
  TextInput,
} from "@/components/ui/primitives";
import { AgentAvatar } from "@/components/domain/avatars";
import {
  EnforcementChip,
  MonoChip,
  ScopeMode,
} from "@/components/domain/labels";
import { ago } from "@/lib/format";
import { cx } from "@/lib/cx";
import {
  AgentStatus,
  Assignment,
  merged,
  money,
  seconds,
  type AgentOverview,
} from "@/components/domain/overview";
import { Stat } from "@/components/domain/data";
import { Content, TopBar } from "@/components/shell/topbar";

const STATUSES = ["all", "running", "waiting", "blocked", "idle"] as const;

export function AgentCard({ overview: o }: { overview: AgentOverview }) {
  const a = o.agent;
  const waiting = o.assignments.filter((w) => w.state === "queued").length;
  return (
    <article className="flex min-w-0 flex-col gap-4 rounded-xl border border-border bg-surface p-[18px] hover:border-border-hover">
      <div className="flex items-center gap-3">
        <AgentAvatar
          name={a.name}
          colorIndex={a.color_index}
          size="lg"
          label={false}
        />
        <div className="min-w-0 flex-1">
          <Link
            href={`/agents/${a.handle}?tab=overview`}
            className="block truncate type-heading hover:text-accent"
          >
            {a.name}
          </Link>
          <p className="truncate text-xs text-fg-secondary">{a.role}</p>
        </div>
        <AgentStatus status={a.status} />
      </div>
      <div className="space-y-2">
        {o.assignments.length ? (
          o.assignments
            .slice(0, 2)
            .map((work) => <Assignment key={work.run_id} work={work} />)
        ) : (
          <div className="rounded-lg border border-dashed border-border-strong p-3">
            <p className="mb-2 text-[13px] text-fg-secondary">
              {o.queue_count
                ? `${o.queue_count} assigned tickets are waiting for dispatch.`
                : "Ready for the next assignment."}
            </p>
            <Link
              href={
                o.queue_count
                  ? `/tickets?view=all&assignee=${a.handle}`
                  : "/tickets?view=all"
              }
              className="text-xs font-medium text-accent"
            >
              {o.queue_count
                ? "Review assigned tickets →"
                : "Assign a ticket →"}
            </Link>
          </div>
        )}
        {o.assignments.length > 2 && (
          <Link
            className="text-xs text-fg-tertiary hover:text-accent"
            href={`/agents/${a.handle}?tab=overview`}
          >
            +{o.assignments.length - 2} other assignments →
          </Link>
        )}
      </div>
      <div
        className="grid grid-cols-4 gap-2"
        title="Last 7 days. Cycle: first In Progress (or creation) to latest Done. Spend: runs started in the window."
      >
        <Stat label="Done 7d" value={o.stats.completed} />
        <Stat label="Merged" value={merged(o.stats)} />
        <Stat label="Cycle" value={seconds(o.stats.median_cycle_seconds)} />
        <Stat label="Spend" value={money(o.stats.spend_usd)} />
      </div>
      <details className="text-xs">
        <summary className="cursor-pointer text-fg-tertiary">
          Definition permissions · {o.scopes.length} scopes
        </summary>
        <div className="mt-3 space-y-2">
          {o.scopes.map((scope) => (
            <div
              key={scope.scope}
              title={scope.how}
              className="flex flex-wrap items-center gap-x-2 gap-y-1"
            >
              <span className="min-w-0 flex-1 font-mono text-[11px]">
                {scope.scope}
              </span>
              <ScopeMode mode={scope.mode} />
              <EnforcementChip level={scope.enforcement} />
            </div>
          ))}
          <p className="pt-1 text-fg-tertiary">
            Project overrides may tighten these permissions. Open the definition
            to inspect them.
          </p>
        </div>
      </details>
      <div className="mt-auto flex flex-wrap items-center gap-2 border-t border-border-subtle pt-3">
        <MonoChip>{a.model}</MonoChip>
        <MonoChip variant="outline">{o.harness}</MonoChip>
        <span className="ml-auto text-xs text-fg-tertiary">
          {o.max_concurrency} slots · {waiting} queued
        </span>
      </div>
      <div className="flex items-center justify-between gap-2 text-xs">
        <Link
          href={`/agents/${a.handle}`}
          className="text-fg-secondary hover:text-accent"
        >
          Edit definition →
        </Link>
        {o.queue_count > 0 && (
          <Link
            href={`/tickets?view=all&assignee=${a.handle}`}
            className="text-fg-tertiary hover:text-accent"
          >
            {o.queue_count} awaiting dispatch →
          </Link>
        )}
      </div>
    </article>
  );
}

/**
 * A handle from a name, valid for the server's rule (2–32 lowercase letters,
 * digits or -, starting with a letter): "Scout 2" → "scout-2".
 */
function handleFrom(name: string) {
  const h = name
    .toLowerCase()
    .normalize("NFKD")
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^[^a-z]+/, "")
    .slice(0, 32)
    .replace(/-+$/, "");
  return h.length >= 2 ? h : "";
}

/** Live assignments and recorded performance, alongside the definition editor. */
export function AgentsScreen() {
  const boot = useBootstrap();
  const roster = useAgentOverviews();
  const [status, setStatus] = useState<string>("all");
  const router = useRouter();
  const create = useCreateAgent();
  const [adding, setAdding] = useState(false);
  const [form, setForm] = useState({ name: "", handle: "" });
  const agents = roster.data?.agents ?? [];
  const visible = agents.filter(
    (o) => status === "all" || o.agent.status === status,
  );
  // An empty Handle field means "use the one derived from the name", which
  // is what its placeholder shows.
  const handle = form.handle.trim() || handleFrom(form.name);

  return (
    <>
      <TopBar
        crumbs={[
          { label: boot.data?.workspace.name ?? "Workspace" },
          { label: "Agents" },
        ]}
        actions={
          <Button variant="primary" icon="plus" onClick={() => setAdding(true)}>
            New agent
          </Button>
        }
      />
      <Content>
        <div className="flex flex-col gap-6">
          <div className="flex flex-col gap-1">
            <h1 className="type-display">Agents</h1>
            <p className="text-sm text-fg-secondary">
              Live assignments, permissions and performance over the last 7
              days.
            </p>
          </div>
          {adding && (
            <form
              className="flex flex-wrap items-end gap-3 rounded-xl border border-border bg-surface p-4"
              onSubmit={(e) => {
                e.preventDefault();
                create.mutate(
                  {
                    name: form.name.trim(),
                    handle,
                    color_index: agents.length % 8,
                  },
                  {
                    onSuccess: (d) => router.push(`/agents/${d.agent.handle}`),
                  },
                );
              }}
            >
              <Field label="Name" htmlFor="na-name">
                <TextInput
                  id="na-name"
                  autoFocus
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  placeholder="Scout"
                />
              </Field>
              <Field label="Handle" htmlFor="na-handle">
                <TextInput
                  id="na-handle"
                  mono
                  prefix="@"
                  value={form.handle}
                  onChange={(e) => setForm({ ...form, handle: e.target.value })}
                  placeholder={handleFrom(form.name) || "scout"}
                />
              </Field>
              <Button
                type="submit"
                variant="primary"
                disabled={!form.name.trim() || !handle || create.isPending}
              >
                Create
              </Button>
              <Button variant="ghost" onClick={() => setAdding(false)}>
                Cancel
              </Button>
              {create.isError && (
                <span
                  role="alert"
                  className="w-full text-xs text-pill-blocked-fg"
                >
                  {create.error.message}
                </span>
              )}
            </form>
          )}

          <div
            className="flex flex-wrap gap-2"
            aria-label="Filter agents by status"
          >
            {STATUSES.map((st) => (
              <button
                key={st}
                onClick={() => setStatus(st)}
                aria-pressed={status === st}
                className={cx(
                  "flex items-center gap-2 rounded-full border px-3 py-1.5 text-xs",
                  status === st
                    ? "border-border-strong bg-selected text-fg"
                    : "border-border text-fg-secondary hover:bg-hover",
                )}
              >
                <span className="capitalize">{st}</span>
                <span className="font-mono text-fg-tertiary">
                  {
                    agents.filter((a) => st === "all" || a.agent.status === st)
                      .length
                  }
                </span>
              </button>
            ))}
          </div>
          {roster.isError ? (
            <Banner
              tone="blocked"
              title="Couldn’t load agents"
              actions={<Button onClick={() => roster.refetch()}>Retry</Button>}
            >
              {roster.error.message}
            </Banner>
          ) : roster.isLoading ? (
            <div
              aria-busy="true"
              className="grid grid-cols-[repeat(auto-fit,minmax(min(100%,320px),1fr))] gap-4"
            >
              {[0, 1, 2].map((i) => (
                <Skeleton key={i} className="h-80 rounded-xl" />
              ))}
            </div>
          ) : !visible.length ? (
            <EmptyState
              icon="agents"
              title={
                agents.length ? "No agents with this status" : "No agents yet"
              }
              actions={
                agents.length ? (
                  <Button onClick={() => setStatus("all")}>
                    Show all agents
                  </Button>
                ) : (
                  <Button onClick={() => setAdding(true)} variant="primary">
                    New agent
                  </Button>
                )
              }
            >
              {agents.length
                ? "Try another status to see the rest of your team."
                : "Create an agent to define its model, instructions and permissions."}
            </EmptyState>
          ) : (
            <div className="grid grid-cols-[repeat(auto-fill,minmax(min(100%,320px),1fr))] gap-4">
              {visible.map((o) => (
                <AgentCard key={o.agent.id} overview={o} />
              ))}
            </div>
          )}
          {roster.data && (
            <p className="text-xs text-fg-tertiary">
              Performance refreshed {ago(roster.data.stats_as_of)}. Queued runs
              wait for runner capacity; assigned tickets require a person to
              dispatch them.
            </p>
          )}
        </div>
      </Content>
    </>
  );
}
