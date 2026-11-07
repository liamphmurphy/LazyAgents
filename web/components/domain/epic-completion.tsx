"use client";

import Link from "next/link";
import { useState, type FormEvent } from "react";
import { ApiError, type Schemas } from "@/lib/api/client";
import { useEpicCriterionMutations, useUpdateEpic } from "@/lib/api/hooks";
import { Button } from "@/components/ui/button";
import { cx } from "@/lib/cx";
import { absoluteTime, ago } from "@/lib/format";

type BoardEpic = Schemas["BoardEpic"];
type Criterion = Schemas["EpicCriterion"];

const STATUS_LABEL: Record<Schemas["EpicStatus"], string> = { open: "Open", done: "Done" };
const inputClass = "h-8 min-w-0 flex-1 rounded-md border border-border-strong bg-surface px-2 text-xs text-fg";
const rowButton = "rounded px-1.5 py-0.5 text-[11px] text-fg-secondary hover:bg-hover hover:text-fg disabled:cursor-not-allowed disabled:opacity-40";

const message = (e: unknown) => (e instanceof Error ? e.message : "Request failed. Try again.");

/**
 * Epic status, points/criteria rollup and the final acceptance criteria
 * editor. Everything renders from `summary` (the board's epic section), so
 * SSE `epic.updated` refetches keep every tab in sync; writes only refetch.
 */
export function EpicCompletion({ summary }: { summary: BoardEpic }) {
  const { epic, criteria, done_blockers: blockers } = summary;
  const status = useUpdateEpic(epic.id);
  const mutations = useEpicCriterionMutations(epic.id);
  const [blocked, setBlocked] = useState<string>();
  const [statusError, setStatusError] = useState<string>();
  const closed = epic.status === "done";

  function changeStatus(next: Schemas["EpicStatus"]) {
    setBlocked(undefined);
    setStatusError(undefined);
    status.mutateAsync({ status: next }).catch((e: unknown) => {
      if (e instanceof ApiError && e.status === 409) setBlocked(e.message);
      else setStatusError(`Couldn't change status: ${message(e)}`);
    });
  }

  const unchecked = criteria.filter((c) => blockers.unchecked_criteria.includes(c.position));
  const gated = blockers.open_tickets.length > 0 || blockers.unchecked_criteria.length > 0;
  const ready = !closed && criteria.length > 0 && !gated;
  return (
    <div className="grid gap-5 md:grid-cols-2">
      <div className="min-w-0 space-y-3">
        <div className="flex flex-wrap items-center gap-2 text-xs">
          <span className="text-fg-secondary">Status: <span className="font-medium text-fg">{STATUS_LABEL[epic.status]}</span></span>
          <span className="text-fg-tertiary">{epic.tickets_total} tickets, {epic.tickets_open} open</span>
        </div>
        <Progress label="Points" done={epic.points_done} total={epic.points_total} value={`${epic.points_done} / ${epic.points_total} pts done`} />
        <Progress label="Criteria" done={epic.criteria_done} total={epic.criteria_total} value={`${epic.criteria_done} / ${epic.criteria_total} met`} />
        <div className="flex flex-wrap items-start gap-3">
          {!closed && (gated || !criteria.length) && <section aria-label="Done blockers" className="min-w-0 flex-1 space-y-1.5 rounded-md border border-border-strong bg-surface-sunken p-3 text-xs">
            <p className="font-semibold text-fg">Can&apos;t mark done yet</p>
            {!criteria.length && <p className="text-fg-secondary">Add acceptance criteria before marking done.</p>}
            {blockers.open_tickets.length > 0 && <div>
              <p className="font-medium text-fg-secondary">Open tickets</p>
              <ul aria-label="Open tickets" className="flex flex-wrap gap-x-3">{blockers.open_tickets.map((key) =>
                <li key={key}><Link href={`/tickets/${encodeURIComponent(key)}`} className="font-mono hover:text-accent">{key} ↗</Link></li>)}</ul>
            </div>}
            {blockers.unchecked_criteria.length > 0 && <div>
              <p className="font-medium text-fg-secondary">Unchecked criteria</p>
              <ul aria-label="Unchecked criteria" className="list-none">{(unchecked.length ? unchecked.map((c) => `${c.position}. ${c.text}`) : blockers.unchecked_criteria.map(String)).map((t) => <li key={t}>{t}</li>)}</ul>
            </div>}
          </section>}
          {!closed && !gated && criteria.length > 0 && <p className="flex-1 text-xs text-fg-tertiary">Ready to mark done</p>}
          {closed
            ? <Button variant="outline" disabled={status.isPending} onClick={() => changeStatus("open")}>{status.isPending ? "Reopening…" : "Reopen epic"}</Button>
            : <Button variant="primary" disabled={!ready || status.isPending} onClick={() => changeStatus("done")}
                title={ready ? undefined : "Close the open tickets and check every criterion first"}>{status.isPending ? "Saving…" : "Mark epic done"}</Button>}
        </div>
        {statusError && <p role="alert" className="text-xs text-pill-blocked-fg">{statusError}</p>}
        {blocked && <div role="alert" aria-label="Epic cannot be marked done" className="space-y-2 rounded-md border border-state-blocked bg-surface p-3 text-xs">
          <div className="flex items-start justify-between gap-3">
            <div>
              <p className="font-semibold text-fg">Epic cannot be marked done</p>
              <p className="text-fg-secondary">Something changed since this page loaded. Epic remains {STATUS_LABEL[epic.status]}.</p>
            </div>
            <button type="button" aria-label="Dismiss completion error" onClick={() => setBlocked(undefined)} className="text-base leading-none">×</button>
          </div>
          <p className="text-fg-tertiary">{blocked}</p>
          <p className="text-fg-secondary">The blockers above are refreshed from the server.</p>
        </div>}
      </div>
      <CriteriaEditor criteria={criteria} closed={closed} mutations={mutations} />
    </div>
  );
}

function CriteriaEditor({ criteria, closed, mutations }: {
  criteria: Criterion[]; closed: boolean; mutations: ReturnType<typeof useEpicCriterionMutations>;
}) {
  const [draft, setDraft] = useState("");
  const [editing, setEditing] = useState<{ id: string; text: string }>();
  const [busy, setBusy] = useState<string>();
  const [ticking, setTicking] = useState<{ id: string; done: boolean }>();
  const [error, setError] = useState<string>();

  function run(id: string, action: string, p: Promise<unknown>) {
    setBusy(id);
    setError(undefined);
    return p.then(() => true, (e: unknown) => {
      setError(`Couldn't ${action}: ${message(e)}`);
      return false;
    }).finally(() => { setBusy(undefined); setTicking(undefined); });
  }
  function add(e: FormEvent) {
    e.preventDefault();
    const text = draft.trim();
    if (!text) return;
    void run("new", "add criterion", mutations.create.mutateAsync(text)).then((ok) => { if (ok) setDraft(""); });
  }
  function save(e: FormEvent) {
    e.preventDefault();
    if (!editing) return;
    const text = editing.text.trim();
    if (!text) return;
    const { id } = editing;
    void run(id, "save criterion", mutations.update.mutateAsync({ id, text })).then((ok) => { if (ok) setEditing(undefined); });
  }

  return (
    <section aria-label="Final acceptance criteria" className="min-w-0">
      <h2 className="mb-2 text-xs font-semibold text-fg-secondary">Final acceptance criteria</h2>
      {criteria.length ? <ol className="space-y-1.5 text-xs">
        {criteria.map((c, i) => {
          const pending = busy === c.id;
          const locked = closed || !!busy;
          return <li key={c.id} aria-busy={pending} className={cx("flex flex-wrap items-start gap-2", pending && "opacity-60")}>
            <input type="checkbox" className="mt-0.5" checked={ticking?.id === c.id ? ticking.done : c.done} aria-label={`Criterion ${c.position} met`}
              disabled={!!busy || (closed && c.done)}
              onChange={(e) => {
                // Show the click while saving; the refetched server value replaces it.
                const done = e.target.checked;
                setTicking({ id: c.id, done });
                void run(c.id, "update criterion", mutations.update.mutateAsync({ id: c.id, done }));
              }} />
            {editing?.id === c.id ? <form onSubmit={save} className="flex min-w-0 flex-1 flex-wrap items-center gap-1.5">
              <span>{c.position}.</span>
              <input autoFocus aria-label={`Criterion ${c.position} text`} value={editing.text} maxLength={1000} className={inputClass}
                onChange={(e) => setEditing({ id: c.id, text: e.target.value })}
                onKeyDown={(e) => { if (e.key === "Escape") setEditing(undefined); }} />
              <Button type="submit" variant="primary" disabled={pending || !editing.text.trim()}>Save</Button>
              <Button variant="ghost" onClick={() => setEditing(undefined)}>Cancel</Button>
            </form> : <>
              <span className={cx("min-w-0 flex-1 break-words", c.done && "text-fg-secondary")}>
                {c.position}. {c.text}
                {c.done && (c.checked_by || c.checked_at) && <span className="block text-[11px] text-fg-tertiary" title={c.checked_at ? absoluteTime(c.checked_at) : undefined}>
                  Checked{c.checked_by && ` by ${c.checked_by.name}`}{c.checked_at && ` ${ago(c.checked_at)}`}
                </span>}
              </span>
              <span className="flex flex-wrap gap-0.5">
                <button type="button" className={rowButton} aria-label={`Move criterion ${c.position} up`} disabled={locked || i === 0}
                  onClick={() => void run(c.id, "reorder criteria", mutations.update.mutateAsync({ id: c.id, position: c.position - 1 }))}>↑</button>
                <button type="button" className={rowButton} aria-label={`Move criterion ${c.position} down`} disabled={locked || i === criteria.length - 1}
                  onClick={() => void run(c.id, "reorder criteria", mutations.update.mutateAsync({ id: c.id, position: c.position + 1 }))}>↓</button>
                <button type="button" className={rowButton} aria-label={`Edit criterion ${c.position}`} disabled={locked}
                  onClick={() => { setError(undefined); setEditing({ id: c.id, text: c.text }); }}>Edit</button>
                <button type="button" className={rowButton} aria-label={`Delete criterion ${c.position}`} disabled={locked}
                  onClick={() => void run(c.id, "delete criterion", mutations.remove.mutateAsync(c.id))}>Delete</button>
              </span>
            </>}
          </li>;
        })}
      </ol> : <p className="text-xs text-fg-secondary">Add final acceptance criteria before this epic can be marked done.</p>}
      {busy && <p role="status" className="sr-only">Saving criteria…</p>}
      {error && <p role="alert" className="mt-2 text-xs text-pill-blocked-fg">{error}</p>}
      {closed ? <p className="mt-2 text-xs text-fg-tertiary">Reopen the epic to edit its criteria.</p>
        : <form onSubmit={add} className="mt-3 flex flex-wrap gap-2">
          <input aria-label="New acceptance criterion" placeholder="Add a final acceptance criterion…" value={draft} maxLength={1000}
            onChange={(e) => setDraft(e.target.value)} className={inputClass} />
          <Button type="submit" variant="outline" disabled={!draft.trim() || busy === "new"}>{busy === "new" ? "Adding…" : "Add criterion"}</Button>
        </form>}
    </section>
  );
}

function Progress({ label, done, total, value }: { label: string; done: number; total: number; value: string }) {
  return <div className="flex items-center gap-3 text-xs">
    <span className="w-11 text-fg-secondary">{label}</span>
    <div role="progressbar" aria-label={label} aria-valuemin={0} aria-valuemax={total || 1} aria-valuenow={done} aria-valuetext={value} className="h-2 max-w-64 flex-1 overflow-hidden rounded-full bg-surface-sunken">
      <div className="h-full rounded-full bg-accent" style={{ width: `${total ? Math.min(100, done / total * 100) : 0}%` }} />
    </div>
    <span className="text-fg-secondary">{value}</span>
  </div>;
}
