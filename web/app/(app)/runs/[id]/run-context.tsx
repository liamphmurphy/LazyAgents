"use client";

// The run page's Context panel (LAZYAGENTS-163), replacing the Details mode:
// the ticket and its epic's progress, acceptance criteria with live ticks, the
// code (branch, diff stat, PR), the run's facts (every column the old list's
// "More columns" had), and buttons to the Raw log, Graph and Files views.

import Link from "next/link";
import { useSyncExternalStore, type ReactNode } from "react";
import type { Schemas } from "@/lib/api/client";
import { useEpicOverviews, useTicket } from "@/lib/api/hooks";
import { cx } from "@/lib/cx";
import { absoluteTime, ago, compact, duration } from "@/lib/format";
import { runnerSlot } from "@/lib/run-list";
import { PURPOSE_LABEL, type RunViewMode } from "@/lib/run-view";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { WorkspaceFolderChip } from "@/components/domain/workspace-folder";
import { CostValue } from "./run-thread";

type RunDetail = Schemas["RunDetail"];

/** Open (the default) or closed, remembered per browser. */
const CONTEXT_KEY = "lazyagents.runs.context";
const CONTEXT_CHANGED = "lazyagents:runs-context";

function subscribe(onChange: () => void) {
  window.addEventListener("storage", onChange);
  window.addEventListener(CONTEXT_CHANGED, onChange);
  return () => {
    window.removeEventListener("storage", onChange);
    window.removeEventListener(CONTEXT_CHANGED, onChange);
  };
}
function read() {
  try {
    return window.localStorage.getItem(CONTEXT_KEY) !== "closed";
  } catch {
    return true;
  }
}

export function useContextPanel(): [boolean, (open: boolean) => void] {
  const open = useSyncExternalStore(subscribe, read, () => true);
  const set = (next: boolean) => {
    try {
      if (next) window.localStorage.removeItem(CONTEXT_KEY);
      else window.localStorage.setItem(CONTEXT_KEY, "closed");
    } catch {
      /* Not remembered. */
    }
    window.dispatchEvent(new Event(CONTEXT_CHANGED));
  };
  return [open, set];
}

function Section({ title, aside, children }: { title: string; aside?: ReactNode; children: ReactNode }) {
  return (
    <section aria-label={title} className="flex flex-col gap-2 border-b border-border-subtle px-4 py-3.5 last:border-b-0">
      <h2 className="flex items-center justify-between text-xs font-medium text-fg-tertiary">
        {title}
        {aside && <span className="font-mono">{aside}</span>}
      </h2>
      {children}
    </section>
  );
}

function Fact({ k, children }: { k: string; children: ReactNode }) {
  return (
    <div className="flex items-baseline justify-between gap-3 text-[12.5px]">
      <dt className="flex-none text-fg-tertiary">{k}</dt>
      <dd className="min-w-0 truncate text-right text-fg-2">{children}</dd>
    </div>
  );
}

function TicketSection({ ticketKey }: { ticketKey: string }) {
  const ticket = useTicket(ticketKey);
  const t = ticket.data;
  const epics = useEpicOverviews(!!t?.epic);
  const epic = t?.epic ? epics.data?.find((e) => e.epic.id === t.epic!.id) : undefined;
  const done = t?.criteria.filter((c) => c.done).length ?? 0;
  return (
    <>
      <Section title="Ticket">
        {!t ? (
          <p className="text-xs text-fg-tertiary">{ticket.isError ? "Couldn’t load the ticket." : "Loading…"}</p>
        ) : (
          <>
            <Link href={`/tickets/${encodeURIComponent(t.key)}`} className="text-[13px] text-fg hover:underline">
              <span className="font-mono text-xs text-fg-secondary">{t.key}</span> {t.title}
            </Link>
            <p className="text-xs text-fg-tertiary">{t.status.replace("_", " ")} · {t.priority}</p>
            {t.epic && (
              <div className="flex flex-col gap-1">
                <p className="flex items-baseline justify-between gap-2 text-xs">
                  <Link href={`/epics/${t.epic.id}`} className="truncate text-fg-2 hover:underline">{t.epic.name}</Link>
                  {epic && <span className="flex-none font-mono text-fg-tertiary">{epic.counts.done}/{epic.counts.total} done</span>}
                </p>
                {epic && epic.counts.total > 0 && (
                  <div role="progressbar" aria-label="Epic progress" aria-valuemin={0} aria-valuemax={epic.counts.total} aria-valuenow={epic.counts.done} className="h-1 overflow-hidden rounded-full bg-surface-sunken">
                    <div className="h-full bg-state-done" style={{ width: `${(epic.counts.done / epic.counts.total) * 100}%` }} />
                  </div>
                )}
              </div>
            )}
          </>
        )}
      </Section>
      {t && t.criteria.length > 0 && (
        <Section title="Acceptance criteria" aside={`${done}/${t.criteria.length}`}>
          <ol className="flex flex-col gap-1.5">
            {[...t.criteria].sort((a, b) => a.position - b.position).map((c) => (
              <li key={c.id} className="flex gap-2 text-[12.5px]">
                <span aria-hidden className={cx("mt-px grid size-3.5 flex-none place-items-center rounded-[4px] border", c.done ? "border-state-done bg-state-done text-on-accent" : "border-border-strong")}>
                  {c.done && <Icon name="check" size={10} strokeWidth={2.4} />}
                </span>
                <span className={cx("min-w-0", c.done ? "text-fg-secondary" : "text-fg-2")}>
                  <span className="sr-only">{c.done ? "Done: " : "Not done: "}</span>
                  {c.text}
                </span>
              </li>
            ))}
          </ol>
        </Section>
      )}
      <CodeSection branch={t?.branch} pr={t?.pr} />
    </>
  );
}

function CodeSection({ branch, pr, runPr }: { branch?: string; pr?: Schemas["PullRequest"]; runPr?: { number?: number; url?: string } }) {
  const number = pr?.number ?? runPr?.number;
  const url = pr?.url ?? runPr?.url;
  if (!branch && !number) return null;
  return (
    <Section title="Code">
      <dl className="flex flex-col gap-1">
        {branch && <Fact k="Branch"><span className="font-mono text-[11.5px]" title={branch}>{branch}</span></Fact>}
        {pr && (pr.additions != null || pr.deletions != null) && (
          <Fact k="Diff">
            <span className="font-mono text-[11.5px]"><span className="text-state-done">+{pr.additions ?? 0}</span> <span className="text-pill-blocked-fg">−{pr.deletions ?? 0}</span></span>
          </Fact>
        )}
        {number && (
          <Fact k="Pull request">
            {url ? <a href={url} target="_blank" rel="noreferrer" className="text-accent hover:underline">#{number}{pr ? ` · ${pr.state}` : ""} ↗</a> : `#${number}`}
          </Fact>
        )}
      </dl>
    </Section>
  );
}

export function RunContext({ d, mode, setMode, onClose }: { d: RunDetail; mode: RunViewMode; setMode: (m: RunViewMode) => void; onClose: () => void }) {
  const run = d.run;
  const slot = runnerSlot(d.events);
  const lastLog = [...d.turns].reverse().find((t) => t.has_log);
  const view = (m: RunViewMode, label: string, icon: "terminal" | "graph" | "folder") => (
    <Button variant={mode === m ? "secondary" : "outline"} icon={icon} aria-pressed={mode === m} onClick={() => setMode(mode === m ? "simple" : m)}>
      {label}
    </Button>
  );
  return (
    <aside aria-label="Context" className="flex w-[320px] flex-none flex-col overflow-y-auto border-l border-border-subtle bg-sidebar/40 max-lg:w-[280px]">
      <div className="flex items-center justify-between px-4 pb-1 pt-3">
        <span className="text-[13px] font-semibold">Context</span>
        <button type="button" onClick={onClose} aria-label="Hide context" title="Hide context" className="grid size-7 place-items-center rounded-md text-fg-tertiary hover:bg-hover hover:text-fg">
          <Icon name="sidebar" size={14} />
        </button>
      </div>
      {run.purpose === "ticket" && run.ticket_key ? <TicketSection ticketKey={run.ticket_key} /> : <CodeSection branch={run.branch} runPr={{ number: run.pr_number, url: run.pr_url }} />}
      <Section title="Run">
        <dl className="flex flex-col gap-1">
          <Fact k="Purpose">{PURPOSE_LABEL[run.purpose] ?? run.purpose}</Fact>
          <Fact k="Model"><span className="font-mono text-[11.5px]">{run.model}</span></Fact>
          <Fact k="Harness"><span className="font-mono text-[11.5px]">{run.harness}</span> · {run.mode}</Fact>
          <Fact k="Started"><span title={absoluteTime(run.started_at)}>{ago(run.started_at)}</span></Fact>
          <Fact k="Elapsed"><span className="font-mono text-[11.5px]">{duration(run.started_at, run.ended_at)}</span></Fact>
          <Fact k="Cost"><span className="font-mono text-[11.5px]"><CostValue d={d} /></span></Fact>
          <Fact k="Tokens"><span className="font-mono text-[11.5px]">{compact(run.input_tokens + run.output_tokens)} · {run.tool_calls} tool calls</span></Fact>
          <Fact k="Sub runs"><span className="font-mono text-[11.5px]">{run.child_count ?? d.child_runs.length}</span></Fact>
          <Fact k="Runner slot"><span className="font-mono text-[11.5px]">{slot ? [slot.runner, slot.backend].filter(Boolean).join(" · ") : run.mode === "unmanaged" ? "none (unmanaged)" : "–"}</span></Fact>
          {run.dispatched_by && <Fact k="Dispatched by">{run.dispatched_by.name}</Fact>}
        </dl>
        {run.workspace_path && <WorkspaceFolderChip path={run.workspace_path} />}
      </Section>
      <Section title="Open">
        <div className="flex flex-wrap gap-1.5">
          {view("details", "Raw log", "terminal")}
          {view("graph", "Graph", "graph")}
          {view("files", "Files", "folder")}
        </div>
        <div className="flex flex-wrap gap-x-3 gap-y-1 text-xs">
          <Link href={`/settings/runner?run_id=${run.id}`} className="text-accent hover:underline">Environment</Link>
          {lastLog && <a href={`/api/v1/runs/${run.id}/turns/${lastLog.seq}/log`} className="text-accent hover:underline">Download raw log</a>}
        </div>
      </Section>
    </aside>
  );
}

/** Shown in the top bar when the panel is closed. */
export function ContextButton({ onOpen }: { onOpen: () => void }) {
  return <Button variant="outline" icon="sidebar" onClick={onOpen}>Context</Button>;
}
