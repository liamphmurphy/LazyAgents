"use client";

import { useSearchParams } from "next/navigation";
import { DropdownMenu } from "radix-ui";
import { useEffect, useMemo, useState } from "react";
import type { AgentDetail } from "@/lib/api/client";
import { useAgent, useBootstrap, useResolved, useSaveDefinition } from "@/lib/api/hooks";
import { cx } from "@/lib/cx";
import { ago, timelineTime } from "@/lib/format";
import { Button, ButtonLink } from "@/components/ui/button";
import { Tabs } from "@/components/ui/controls";
import { Icon } from "@/components/ui/icon";
import { Banner, EmptyState, Skeleton } from "@/components/ui/primitives";
import { AgentAvatar } from "@/components/domain/avatars";
import { MonoChip, Pill, type PillTone } from "@/components/domain/labels";
import { floatPanel } from "@/components/domain/pickers";
import { TopBar } from "@/components/shell/topbar";
import { DefinitionEditor, describeChanges, draftOf, type Draft } from "./definition-editor";
import { OverrideDialog } from "./override-dialog";
import { OverviewTab, RunsTab } from "./overview-tab";
import { TokensTab } from "./tokens-tab";

const STATUS: Record<AgentDetail["agent"]["status"], { tone: PillTone; label: string }> = {
  running: { tone: "done", label: "Running" },
  waiting: { tone: "human", label: "Waiting" },
  blocked: { tone: "blocked", label: "Blocked" },
  idle: { tone: "neutral", label: "Idle" },
};

function ResolvedPanel({ detail }: { detail: AgentDetail }) {
  const boot = useBootstrap();
  const projects = boot.data?.projects ?? [];
  const [project, setProject] = useState<string | undefined>(detail.overrides[0]?.project.key ?? projects[0]?.key);
  const r = useResolved(detail.agent.handle, project);
  const p = projects.find((x) => x.key === project);
  const fromProject = (f: string) => r.data?.provenance[f] === "project" && <MonoChip variant="outline">project</MonoChip>;
  const extraLines = r.data ? r.data.instructions.trim().split("\n").length - detail.spec.instructions.trim().split("\n").length : 0;

  return (
    <section className="flex min-w-0 flex-col gap-2 rounded-xl border border-border bg-surface p-4">
      <div className="flex items-center gap-2">
        <h2 className="text-[13px] font-semibold">Resolved for</h2>
        <DropdownMenu.Root>
          <DropdownMenu.Trigger asChild>
            <button type="button" className="flex h-7 flex-1 items-center gap-2 rounded-md border border-border-strong px-2 text-[12.5px] hover:bg-hover">
              {p && <span aria-hidden className="size-2.5 rounded-[3px]" style={{ background: p.color }} />}
              <span className="flex-1 truncate text-left">{p?.name ?? "Pick a project"}</span>
              <Icon name="chev" size={12} className="text-fg-tertiary" />
            </button>
          </DropdownMenu.Trigger>
          <DropdownMenu.Portal>
            <DropdownMenu.Content align="end" sideOffset={4} className={cx(floatPanel, "w-56")}>
              {projects.map((x) => (
                <DropdownMenu.Item key={x.key} onSelect={() => setProject(x.key)} className="flex h-8 items-center gap-2.5 rounded-md px-2 outline-none data-[highlighted]:bg-hover">
                  <span aria-hidden className="size-2.5 rounded-[3px]" style={{ background: x.color }} />
                  {x.name}
                </DropdownMenu.Item>
              ))}
            </DropdownMenu.Content>
          </DropdownMenu.Portal>
        </DropdownMenu.Root>
      </div>
      {!r.data ? (
        <Skeleton className="h-24" />
      ) : (
        <dl className="grid grid-cols-[84px_minmax(0,1fr)] items-center gap-x-3 gap-y-2 text-[12.5px]">
          <dt className="text-fg-tertiary">Harness</dt>
          <dd className="flex items-center gap-1.5"><MonoChip>{r.data.harness}</MonoChip>{fromProject("harness")}</dd>
          <dt className="text-fg-tertiary">Model</dt>
          <dd className="flex items-center gap-1.5"><MonoChip>{r.data.model}</MonoChip>{fromProject("model")}</dd>
          <dt className="text-fg-tertiary">Instructions</dt>
          <dd className="flex items-center gap-1.5">
            {r.data.provenance.instructions ? `Agent + ${extraLines} line${extraLines === 1 ? "" : "s"}` : "Agent's"}
            {fromProject("instructions")}
          </dd>
          <dt className="text-fg-tertiary">MCP</dt>
          <dd className="flex min-w-0 items-center gap-1.5">
            <span className="truncate font-mono text-xs">{["lazyagents", ...r.data.mcp_servers.map((m) => m.name)].join(", ")}</span>
            {fromProject("mcp_servers")}
          </dd>
          <dt className="text-fg-tertiary">Approval</dt>
          <dd className="flex flex-wrap gap-1">
            {r.data.scopes.filter((s) => s.mode === "requires_approval").map((s) => (
              <span key={s.scope} className="rounded-[5px] bg-pill-human-bg px-1.5 py-0.5 font-mono text-[11px] text-pill-human-fg">{s.scope}</span>
            ))}
          </dd>
          <dt className="text-fg-tertiary">Denied</dt>
          <dd className="flex flex-wrap gap-1">
            {r.data.scopes.filter((s) => s.mode === "denied").map((s) => (
              <span key={s.scope} className="rounded-[5px] bg-pill-blocked-bg px-1.5 py-0.5 font-mono text-[11px] text-pill-blocked-fg">
                {s.scope}
                {s.source === "project" && " ·p"}
              </span>
            ))}
          </dd>
        </dl>
      )}
      <p className="text-[11.5px] text-fg-tertiary">Each run stores a snapshot of this, so editing {detail.agent.name} never changes a run in progress.</p>
    </section>
  );
}

function OverridesPanel({ detail, onEdit }: { detail: AgentDetail; onEdit: (key: string) => void }) {
  const boot = useBootstrap();
  const without = (boot.data?.projects ?? []).filter((p) => !detail.overrides.some((o) => o.project.key === p.key));
  return (
    <section className="flex flex-col gap-1">
      <div className="flex items-center justify-between px-1">
        <h2 className="text-[12.5px] text-fg-tertiary">Project overrides</h2>
        {without.length > 0 && (
          <DropdownMenu.Root>
            <DropdownMenu.Trigger asChild>
              <Button variant="ghost" icon="plus" className="h-7 px-2 text-xs text-fg-secondary">Add</Button>
            </DropdownMenu.Trigger>
            <DropdownMenu.Portal>
              <DropdownMenu.Content align="end" sideOffset={4} className={cx(floatPanel, "w-56")}>
                {without.map((p) => (
                  <DropdownMenu.Item key={p.key} onSelect={() => onEdit(p.key)} className="flex h-8 items-center gap-2.5 rounded-md px-2 outline-none data-[highlighted]:bg-hover">
                    <span aria-hidden className="size-2.5 rounded-[3px]" style={{ background: p.color }} />
                    {p.name}
                  </DropdownMenu.Item>
                ))}
              </DropdownMenu.Content>
            </DropdownMenu.Portal>
          </DropdownMenu.Root>
        )}
      </div>
      {detail.overrides.map((o) => (
        <button key={o.project.key} type="button" onClick={() => onEdit(o.project.key)} className="flex flex-col gap-0.5 rounded-lg px-3 py-2 text-left hover:bg-hover">
          <span className="flex items-center gap-2 text-[13px] font-medium">
            <span aria-hidden className="size-2.5 rounded-[3px]" style={{ background: o.project.color }} />
            {o.project.name}
            <span className="ml-auto font-mono text-[11px] font-normal text-fg-tertiary">
              {o.changes.length} change{o.changes.length === 1 ? "" : "s"}
            </span>
          </span>
          {o.changes.map((c) => (
            <span key={c} className="truncate pl-[18px] text-xs text-fg-secondary">{c}</span>
          ))}
        </button>
      ))}
      {without.slice(0, 2).map((p) => (
        <div key={p.key} className="flex flex-col gap-0.5 px-3 py-2">
          <span className="flex items-center gap-2 text-[13px]">
            <span aria-hidden className="size-2.5 rounded-[3px]" style={{ background: p.color }} />
            {p.name}
            <span className="ml-auto text-fg-tertiary">–</span>
          </span>
          <span className="pl-[18px] text-xs text-fg-tertiary">Uses the agent definition as is</span>
        </div>
      ))}
    </section>
  );
}

export function AgentScreen({ handle }: { handle: string }) {
  const { data: detail, isError, error } = useAgent(handle);
  const requestedTab = useSearchParams().get("tab") ?? "definition";
 const tab = ["tokens", "overview", "runs"].includes(requestedTab) ? requestedTab : "definition";
  const boot = useBootstrap();
  const save = useSaveDefinition(handle);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [editing, setEditing] = useState<string | null>(null);
  const base = useMemo(() => (detail ? draftOf(detail) : null), [detail]);
  const current = draft ?? base;
  const dirty = !!draft && !!base && JSON.stringify(draft) !== JSON.stringify(base);
  const nextVersion = (detail?.definition.version ?? 0) + 1;

  function doSave() {
    if (!dirty || !draft || !base || save.isPending) return;
    save.mutate({ ...draft, note: describeChanges(base, draft) }, { onSuccess: () => setDraft(null) });
  }

  // ⌘S saves the definition (DESIGN.md §6).
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (tab === "definition" && (e.metaKey || e.ctrlKey) && e.key === "s") {
        e.preventDefault();
        doSave();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  // Warn before leaving with unsaved changes.
  useEffect(() => {
    if (!dirty) return;
    const onBeforeUnload = (e: BeforeUnloadEvent) => e.preventDefault();
    window.addEventListener("beforeunload", onBeforeUnload);
    return () => window.removeEventListener("beforeunload", onBeforeUnload);
  }, [dirty]);

  if (isError) {
    const missing = "status" in error && error.status === 404;
    return (
      <>
        <TopBar crumbs={[{ label: "Agents", href: "/agents" }, { label: `@${handle}` }]} />
        {missing ? (
          <EmptyState icon="agents" title={`No agent @${handle}`} actions={<ButtonLink href="/agents" variant="outline">All agents</ButtonLink>} />
        ) : (
          <div className="p-6"><Banner tone="blocked" title="Couldn't load this agent">{error.message}</Banner></div>
        )}
      </>
    );
  }

  const editingProject = editing ? boot.data?.projects.find((p) => p.key === editing) : undefined;

  return (
    <>
      <TopBar
        crumbs={[{ label: boot.data?.workspace.name ?? "Workspace" }, { label: "Agents", href: "/agents" }, { label: detail?.agent.name ?? `@${handle}` }]}
        actions={
          detail && (
            <>
              {dirty && (
                <span className="flex items-center gap-1.5 text-[12.5px] text-pill-human-fg">
                  <span aria-hidden className="size-1.5 rounded-full bg-state-human" /> Unsaved changes
                </span>
              )}
              {save.isError && <span role="alert" className="text-xs text-pill-blocked-fg">{save.error.message}</span>}
              {dirty && <Button variant="ghost" onClick={() => setDraft(null)}>Discard</Button>}
              {tab === "definition" && (
                <Button variant="primary" disabled={!dirty || save.isPending} onClick={doSave} hint="⌘S">
                  Save as v{nextVersion}
                </Button>
              )}
            </>
          )
        }
      />
      {!detail || !current ? (
        <div aria-busy="true" className="flex flex-col gap-4 px-7 py-8">
          <Skeleton className="h-10 w-72" />
          <Skeleton className="h-64" />
        </div>
      ) : (
        <>
          <div className="flex flex-wrap items-center justify-between gap-4 border-b border-border-subtle px-7 py-5">
            <div className="flex items-center gap-3.5">
              <AgentAvatar name={current.name || detail.agent.name} colorIndex={current.color_index} size="lg" label={false} />
              <div className="flex flex-col gap-0.5">
                <div className="flex items-center gap-2.5">
                  <h1 className="type-display">{detail.agent.name}</h1>
                  <Pill tone={STATUS[boot.data?.agents.find(a => a.handle === handle)?.status ?? detail.agent.status].tone}>{STATUS[boot.data?.agents.find(a => a.handle === handle)?.status ?? detail.agent.status].label}</Pill>
                </div>
                <span className="text-[13px] text-fg-secondary">
                  {detail.agent.role} · definition <b className="font-mono font-medium text-fg">v{detail.definition.version}</b>
                  {detail.definition.created_by && ` saved by ${detail.definition.created_by.name}`} {ago(detail.definition.created_at)}
                </span>
              </div>
            </div>
            <Tabs
              label="Agent sections"
              value={tab}
              items={[
                { key: "overview", label: "Overview", href: `/agents/${handle}?tab=overview` },
                { key: "runs", label: "Runs", href: `/agents/${handle}?tab=runs` },
                { key: "definition", label: "Definition", href: `/agents/${handle}` },
                { key: "overrides", label: "Project overrides", count: detail.overrides.length, href: `/agents/${handle}#overrides` },
                { key: "tokens", label: "Tokens", href: `/agents/${handle}?tab=tokens` },
              ]}
            />
          </div>
          <div className="flex flex-1 flex-wrap">
            <div className="min-w-0 flex-[999_1_560px] px-7 py-6 max-sm:px-4">
              <div className="mx-auto max-w-[760px]">
                {tab === "overview" ? <OverviewTab handle={handle} /> : tab === "runs" ? <RunsTab handle={handle} /> : tab === "tokens" ? <TokensTab detail={detail} /> : <DefinitionEditor detail={detail} draft={current} onChange={setDraft} />}
              </div>
            </div>
            <aside aria-label="Overrides and versions" className="flex min-w-0 flex-[1_1_300px] flex-col gap-5 border-l border-border-subtle bg-panel px-4 py-6 lg:max-w-[340px]">
              <div id="overrides">
                <OverridesPanel detail={detail} onEdit={setEditing} />
              </div>
              <ResolvedPanel detail={detail} />
              <section className="flex flex-col gap-1">
                <h2 className="px-1 text-[12.5px] text-fg-tertiary">Versions</h2>
                <ol>
                  {detail.versions.map((v) => (
                    <li key={v.id} className="flex items-baseline gap-3 px-1 py-1.5 text-[13px]">
                      <span className="w-6 font-mono text-xs text-fg-secondary">v{v.version}</span>
                      <span className="flex-1 truncate" title={v.note}>{v.note || "—"}</span>
                      <span className="text-xs text-fg-tertiary">{timelineTime(v.created_at)}</span>
                    </li>
                  ))}
                </ol>
              </section>
            </aside>
          </div>
        </>
      )}
      {editing && detail && editingProject && <OverrideDialog detail={detail} project={editingProject} onOpenChange={(o) => !o && setEditing(null)} />}
    </>
  );
}
