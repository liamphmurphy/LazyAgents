"use client";

import { spendCost } from "@/lib/format";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState, type ReactNode } from "react";
import {
  useCreateAgent,
  useAgentOverviews,
} from "@/lib/api/hooks";
import { Button } from "@/components/ui/button";
import { Segmented } from "@/components/ui/controls";
import type { AgentKind } from "@/lib/api/client";
import {
  Banner,
  EmptyState,
  Field,
  Skeleton,
  TextInput,
} from "@/components/ui/primitives";
import { AgentAvatar } from "@/components/domain/avatars";
import { Flag, MonoChip } from "@/components/domain/labels";
import { ago } from "@/lib/format";
import { countModes, liveCounts } from "@/lib/agent-scopes";
import { cx } from "@/lib/cx";
import {
  AgentStatus,
  PlanningAssignment,
  type AgentOverview,
} from "@/components/domain/overview";
import { Icon } from "@/components/ui/icon";
import { useMarkAgentsReviewed } from "@/components/domain/setup";

const STATUSES = ["all", "running", "waiting", "blocked", "idle"] as const;
const KINDS = [
  { value: "all", label: "All kinds" },
  { value: "main", label: "Main" },
  { value: "sub", label: "Sub" },
] as const;

function CardStat({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="flex min-w-0 flex-col gap-0.5">
      <dt className="truncate text-12 text-fg-tertiary">{label}</dt>
      <dd className="font-mono text-15 font-medium text-fg">{value}</dd>
    </div>
  );
}

/** One live ticket run: key, title and where it is. */
function LiveWork({ work }: { work: AgentOverview["assignments"][number] }) {
  const step = work.steps_total > 0 ? `Step ${work.step_position || work.steps_done}/${work.steps_total} · ` : "";
  return (
    <Link href={`/runs/${work.run_id}`} className="flex flex-col gap-0.5 rounded-lg border border-border-subtle bg-surface-sunken px-3 py-2.5 hover:border-border-hover">
      <span className="flex min-w-0 items-baseline gap-2">
        <span className="flex-none font-mono text-12 text-fg-secondary">{work.ticket.key}</span>
        <span className="min-w-0 truncate text-13 text-fg-2">{work.ticket.title}</span>
      </span>
      <span className="truncate text-12 text-fg-tertiary">
        {step}
        {work.step || work.state.replaceAll("_", " ")}
      </span>
    </Link>
  );
}

export function AgentCard({ overview: o }: { overview: AgentOverview }) {
  const a = o.agent;
  const sub = a.kind === "sub";
  const { live, open } = liveCounts(o);
  const modes = countModes(o.scopes);
  const href = `/agents/${a.handle}`;
  return (
    <article aria-label={a.name} className="flex min-w-0 flex-col gap-4 rounded-xl border border-border bg-surface p-[18px] hover:border-border-hover">
      <div className="flex items-center gap-3">
        <AgentAvatar name={a.name} colorIndex={a.color_index} size="lg" label={false} />
        <div className="min-w-0 flex-1">
          <Link href={`${href}?tab=overview`} className="block truncate type-heading hover:text-accent">
            {a.name}
          </Link>
          <p className="truncate text-12 text-fg-secondary">
            {sub ? "Sub agent" : "Main"}
            {a.role && ` · ${a.role}`}
          </p>
        </div>
        <AgentStatus status={a.status} />
      </div>
      <div className="flex flex-wrap items-center gap-1.5">
        <MonoChip>{a.model}</MonoChip>
        <MonoChip variant="outline">{o.harness}</MonoChip>
        <span className="font-mono text-11 text-fg-tertiary">@{a.handle}</span>
      </div>
      <dl className="grid grid-cols-4 gap-2 border-y border-border-subtle py-3" title="Live runs include planning runs. Done and spend cover the last 7 days.">
        <CardStat label="Live runs" value={live} />
        <CardStat label="Open tickets" value={sub ? "—" : open} />
        <CardStat label="Done · 7d" value={sub ? "—" : o.stats.completed} />
        <CardStat label="Spend · 7d" value={spendCost(o.stats.spend_usd, o.stats.unpriced_runs)} />
      </dl>
      <div className="space-y-2">
        {o.planning_runs.map((work) => (
          <PlanningAssignment key={work.id} work={work} />
        ))}
        {o.assignments.slice(0, 1).map((work) => (
          <LiveWork key={work.run_id} work={work} />
        ))}
        {o.assignments.length > 1 && (
          <Link className="block text-12 text-fg-tertiary hover:text-accent" href={`${href}?tab=overview`}>
            +{o.assignments.length - 1} more live run{o.assignments.length > 2 ? "s" : ""} →
          </Link>
        )}
        {!live && (
          <p className="rounded-lg border border-dashed border-border-strong px-3 py-2.5 text-13 text-fg-secondary">
            {sub
              ? "Runs only when a main agent delegates a task to it. Sub agents can't take tickets."
              : o.queue_count
                ? `${o.queue_count} assigned ticket${o.queue_count === 1 ? " is" : "s are"} waiting for dispatch.`
                : "Ready for the next assignment."}
          </p>
        )}
      </div>
      <div className="flex flex-wrap items-center gap-1.5 text-12">
        <span className="mr-1 text-fg-tertiary">Scopes</span>
        {modes.allowed > 0 && <Flag tone="done">{modes.allowed} allowed</Flag>}
        {modes.requires_approval > 0 && <Flag tone="human">{modes.requires_approval} approval</Flag>}
        {modes.denied > 0 && <Flag tone="blocked">{modes.denied} denied</Flag>}
        {!o.scopes.length && <span className="text-fg-tertiary">none granted</span>}
        <Link href={`${href}?tab=scopes`} className="ml-auto text-fg-secondary hover:text-accent">
          Scopes →
        </Link>
      </div>
      <div className="mt-auto flex items-center justify-between gap-2 border-t border-border-subtle pt-3 text-12">
        <Link href={`${href}?tab=overview`} className="font-medium text-accent">
          Open agent →
        </Link>
        {!sub && o.queue_count > 0 && (
          <Link href={`/tickets?view=all&assignee=${a.handle}`} className="text-fg-tertiary hover:text-accent">
            {o.queue_count} awaiting dispatch →
          </Link>
        )}
        {!sub && !o.queue_count && !live && (
          <Link href="/tickets?view=all" className="text-fg-tertiary hover:text-accent">
            Assign a ticket →
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

/**
 * Live assignments and recorded performance, alongside the definition editor.
 * Shown in Customization's pane, under the Agents group, next to Personas.
 */
export function AgentsSection() {
  useMarkAgentsReviewed();
  const roster = useAgentOverviews();
  const [status, setStatus] = useState<string>("all");
  const [kind, setKind] = useState<(typeof KINDS)[number]["value"]>("all");
  const router = useRouter();
  const create = useCreateAgent();
  const [adding, setAdding] = useState(false);
  const [form, setForm] = useState<{ name: string; handle: string; kind: AgentKind }>({ name: "", handle: "", kind: "main" });
  const agents = roster.data?.agents ?? [];
  const ofKind = agents.filter((o) => kind === "all" || o.agent.kind === kind);
  const visible = ofKind.filter(
    (o) => status === "all" || o.agent.status === status,
  );
  // An empty Handle field means "use the one derived from the name", which
  // is what its placeholder shows.
  const handle = form.handle.trim() || handleFrom(form.name);

  return (
    <div className="flex flex-col gap-6">
      <header className="flex items-center gap-4">
        <span className="grid size-[72px] flex-none place-items-center rounded-2xl bg-selected text-fg-secondary"><Icon name="agents" size={28} /></span>
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <h2 className="truncate type-display text-fg">Agents</h2>
          <p className="text-14 text-fg-secondary">
            Agents run your tickets; personas talk with you. A Start card hands a ticket to its agent, and the agent&apos;s scopes, budget and project overrides decide what the run may do.
          </p>
        </div>
        <Button variant="primary" icon="plus" onClick={() => setAdding(true)}>
          New agent
        </Button>
      </header>
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
                kind: form.kind,
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
          <div className="flex flex-col gap-1.5">
            <span className="text-13 font-medium text-fg-2">Kind</span>
            <Segmented
              label="Agent kind"
              value={form.kind}
              onChange={(kind) => setForm({ ...form, kind })}
              options={[
                { value: "main", label: "Main" },
                { value: "sub", label: "Sub" },
              ]}
            />
          </div>
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
              className="w-full text-12 text-pill-blocked-fg"
            >
              {create.error.message}
            </span>
          )}
        </form>
      )}

      <div className="flex flex-wrap items-center gap-2">
        <div
          role="group"
          className="flex flex-wrap gap-2"
          aria-label="Filter agents by status"
        >
          {STATUSES.map((st) => (
            <button
              key={st}
              onClick={() => setStatus(st)}
              aria-pressed={status === st}
              className={cx(
                "flex items-center gap-2 rounded-full border px-3 py-1.5 text-12",
                status === st
                  ? "border-border-strong bg-selected text-fg"
                  : "border-border text-fg-secondary hover:bg-hover",
              )}
            >
              <span className="capitalize">{st}</span>
              <span className="font-mono text-fg-tertiary">
                {
                  ofKind.filter((a) => st === "all" || a.agent.status === st)
                    .length
                }
              </span>
            </button>
          ))}
        </div>
        <span className="ml-auto">
          <Segmented label="Filter agents by kind" value={kind} onChange={setKind} options={KINDS} />
        </span>
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
            agents.length ? "No agents match these filters" : "No agents yet"
          }
          actions={
            agents.length ? (
              <Button onClick={() => { setStatus("all"); setKind("all"); }}>
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
            ? "Try another status or kind to see the rest of your team."
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
        <p className="text-12 text-fg-tertiary">
          Live runs include planning runs. Open tickets are assigned tickets
          awaiting dispatch plus tickets with a live run. Spend counts runs
          started in the last 7 days; refreshed{" "}
          {ago(roster.data.stats_as_of)}.
        </p>
      )}
    </div>
  );
}
