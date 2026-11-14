"use client";

import Link from "next/link";
import { Dialog } from "radix-ui";
import { useEffect, useState, type ReactNode } from "react";
import { ApiError, type Bootstrap, type Priority, type Ticket, type TicketPatch, type TicketStatus } from "@/lib/api/client";
import { useSaveTicketDraft } from "@/lib/api/board-hooks";
import {
  ESTIMATE_SCALE, buildPatch, classifyError, draftFromSnapshot, moveCriterion, newCriterion, snapshotFromTicket,
  type EditorErrors, type TicketDraft,
} from "@/lib/ticket-draft";
import { Button } from "@/components/ui/button";
import { Banner, Field, Skeleton, TextArea, TextInput } from "@/components/ui/primitives";
import { Markdown } from "./markdown";
import { PRIORITY_NAME, PRIORITY_ORDER, STATUS_NAME } from "./meta";
import { confirmAction } from "@/components/ui/confirm";

const BOARD_STATUS_ORDER: TicketStatus[] = ["backlog", "todo", "in_progress", "in_review", "done", "canceled"];
const selectClass = "h-[30px] w-full rounded-md border border-border-strong bg-surface px-2 text-13 text-fg focus:border-accent focus:outline-none";
const iconBtn = "h-7 w-7 rounded px-0 text-center hover:bg-hover disabled:opacity-40";

function FieldError({ id, message }: { id: string; message?: string }) {
  return message ? <p id={id} role="alert" className="text-12 text-pill-blocked-fg">{message}</p> : null;
}

/**
 * Click-to-edit ticket modal. The baseline is the detail taken when the modal
 * opened (or was reloaded); later refetches never touch the draft.
 */
export function TicketEditorDialog({ ticketKey, detail, bootstrap, onClose, onReload }: {
  ticketKey: string;
  detail?: Ticket;
  bootstrap: Bootstrap;
  onClose: () => void;
  onReload: () => Promise<Ticket | undefined>;
}) {
  const [opener] = useState(() => document.activeElement instanceof HTMLElement ? document.activeElement : null);
  const [dirty, setDirty] = useState(false);
  const close = async () => {
    if (!dirty || await confirmAction({ title: "Discard your unsaved changes?", confirmLabel: "Discard", cancelLabel: "Keep editing" })) onClose();
  };
  const [loaded, setLoaded] = useState<{ ticket: Ticket; nonce: number }>();
  if (detail && !loaded) setLoaded({ ticket: detail, nonce: 0 });

  return (
    <Dialog.Root open onOpenChange={(open) => { if (!open) close(); }}>
      <Dialog.Portal>
        <Dialog.Overlay className="motion-scrim fixed inset-0 z-40 bg-scrim" />
        <Dialog.Content
          aria-describedby="ticket-editor-desc"
          onCloseAutoFocus={(e) => {
            e.preventDefault();
            // On a direct link nothing opened it: focus is still on the body.
            if (opener?.isConnected && opener !== document.body) opener.focus();
            else document.querySelector<HTMLElement>(`[data-ticket-key="${CSS.escape(ticketKey)}"]`)?.focus();
          }}
          className="motion-dialog fixed left-1/2 top-[6vh] z-50 flex max-h-[88vh] w-[min(720px,calc(100%-32px))] -translate-x-1/2 flex-col overflow-hidden rounded-2xl border border-border-float bg-surface-raised shadow-[var(--shadow-float)] outline-none"
        >
          <div className="border-b border-border-subtle px-[18px] py-3.5">
            <Dialog.Title className="type-heading">Edit <span className="font-mono">{ticketKey}</span></Dialog.Title>
            <p id="ticket-editor-desc" className="text-13 text-fg-secondary">Changes are saved when you choose Save changes.</p>
          </div>
          {loaded ? (
            <EditorForm
              key={loaded.nonce}
              ticket={loaded.ticket}
              bootstrap={bootstrap}
              onClose={onClose}
              onFetch={onReload}
              onCancel={close}
              onDirty={setDirty}
              onReload={async () => {
                const fresh = await onReload();
                if (fresh) setLoaded((l) => ({ ticket: fresh, nonce: (l?.nonce ?? 0) + 1 }));
                return !!fresh;
              }}
            />
          ) : (
            <div aria-busy="true" role="status" className="flex flex-col gap-2 px-[18px] py-4">
              <span className="sr-only">Loading ticket…</span>
              <Skeleton className="h-3 w-2/3" /><Skeleton className="h-24 w-full" /><Skeleton className="h-3 w-1/2" />
            </div>
          )}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

function EditorForm({ ticket, bootstrap, onClose, onCancel, onDirty, onReload, onFetch }: {
  ticket: Ticket; bootstrap: Bootstrap; onClose: () => void; onCancel: () => void; onDirty: (dirty: boolean) => void; onReload: () => Promise<boolean>; onFetch: () => Promise<Ticket | undefined>;
}) {
  const [base, setBase] = useState(() => snapshotFromTicket(ticket));
  const estimatedBy = ticket.estimated_by;
  const [draft, setDraft] = useState<TicketDraft>(() => draftFromSnapshot(base));
  const [errors, setErrors] = useState<EditorErrors>({ conflict: false, fields: {} });
  const [busy, setBusy] = useState(false);
  const [preview, setPreview] = useState(false);
  const save = useSaveTicketDraft(ticket.key);
  const patch = buildPatch(base, draft);
  useEffect(() => onDirty(!!patch), [patch, onDirty]);
  const set = <K extends keyof TicketDraft>(k: K, v: TicketDraft[K]) => setDraft((d) => ({ ...d, [k]: v }));

  const agents = bootstrap.agents.filter((a) => a.kind === "main" || a.id === base.assignee_agent_id);
  const epics = bootstrap.epics.filter((e) => e.project_id === ticket.project.id || e.id === base.epic_id);
  const scale: number[] = [...ESTIMATE_SCALE];
  if (draft.estimate_points != null && !scale.includes(draft.estimate_points)) scale.push(draft.estimate_points);
  const estimator = estimatedBy?.type === "agent"
    ? `@${bootstrap.agents.find((a) => a.id === estimatedBy.id)?.handle ?? estimatedBy.name}`
    : estimatedBy?.name;

  async function submit() {
    if (!patch || busy) return;
    setBusy(true);
    setErrors({ conflict: false, fields: {} });
    try {
      await save(patch as TicketPatch);
      onClose();
    } catch (e) {
      setErrors(e instanceof ApiError ? classifyError(e.status, e.field, e.message) : { conflict: false, fields: {}, general: e instanceof Error ? e.message : "Couldn't save the ticket." });
      setBusy(false);
    }
  }

  async function reapplyMine() {
    if (!patch || busy) return;
    setBusy(true);
    try {
      const fresh = await onFetch();
      if (!fresh) throw new Error("Couldn't reload the ticket. Try again.");
      const nextBase = snapshotFromTicket(fresh);
      const nextDraft = draftFromSnapshot(nextBase);
      for (const field of ["title", "description_md", "status", "priority", "estimate_points", "assignee_agent_id", "epic_id"] as const) {
        if (field in patch) Object.assign(nextDraft, { [field]: draft[field] });
      }
      if (patch.criteria) nextDraft.criteria = draft.criteria;
      setBase(nextBase);
      setDraft(nextDraft);
      setErrors({ conflict: false, fields: {} });
    } catch (e) {
      setErrors({ conflict: true, fields: {}, general: e instanceof Error ? e.message : "Couldn't reload the ticket." });
    } finally { setBusy(false); }
  }

  async function reload() {
    setBusy(true);
    try {
      if (!(await onReload())) setErrors({ conflict: true, fields: {}, general: "Couldn't reload the ticket. Try again." });
    } finally {
      setBusy(false);
    }
  }

  const err = errors.fields;
  const criteria = draft.criteria;
  const setCriteria = (next: TicketDraft["criteria"]) => set("criteria", next);

  return (
    <>
      <form
        id="ticket-editor-form"
        onSubmit={(e) => { e.preventDefault(); void submit(); }}
        onKeyDown={(e) => { if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) { e.preventDefault(); void submit(); } }}
        className="flex flex-col gap-4 overflow-y-auto px-[18px] py-4"
      >
        {errors.conflict && (
          <Banner tone="blocked" title="This ticket changed by someone else. Your unsaved edits are still shown."
            actions={<><Button type="button" variant="outline" disabled={busy} onClick={() => void reapplyMine()}>Reapply mine</Button><Button type="button" variant="outline" disabled={busy} onClick={() => void reload()}>Reload latest ticket (replaces this draft)</Button></>} />
        )}
        {errors.general && <p role="alert" className="text-13 text-pill-blocked-fg">{errors.general}</p>}
        <Field label="Title" htmlFor="editor-title">
          <TextInput id="editor-title" value={draft.title} aria-invalid={!!err.title} aria-describedby={err.title ? "err-title" : undefined} onChange={(e) => set("title", e.target.value)} />
          <FieldError id="err-title" message={err.title} />
        </Field>
        <Field label="Description · Markdown" htmlFor="editor-description">
          <div className="flex gap-2 text-12"><button type="button" aria-pressed={!preview} onClick={() => setPreview(false)}>Write</button><button type="button" aria-pressed={preview} onClick={() => setPreview(true)}>Preview</button></div>
          {preview ? <Markdown>{draft.description_md || "_Nothing to preview._"}</Markdown> : <TextArea id="editor-description" rows={5} value={draft.description_md} aria-invalid={!!err.description_md} aria-describedby={err.description_md ? "err-description" : undefined} onChange={(e) => set("description_md", e.target.value)} />}
          <FieldError id="err-description" message={err.description_md} />
        </Field>
        <div className="grid grid-cols-2 gap-4 max-sm:grid-cols-1">
          <Field label="Status" htmlFor="editor-status">
            <select id="editor-status" className={selectClass} value={draft.status} onChange={(e) => set("status", e.target.value)}>
              {BOARD_STATUS_ORDER.map((s) => <option key={s} value={s}>{STATUS_NAME[s]}</option>)}
            </select>
            <FieldError id="err-status" message={err.status} />
          </Field>
          <Field label="Priority" htmlFor="editor-priority">
            <select id="editor-priority" className={selectClass} value={draft.priority} onChange={(e) => set("priority", e.target.value)}>
              {PRIORITY_ORDER.map((p: Priority) => <option key={p} value={p}>{PRIORITY_NAME[p]}</option>)}
            </select>
            <FieldError id="err-priority" message={err.priority} />
          </Field>
          <Field label="Point estimate" htmlFor="editor-estimate"
            hint={draft.estimate_points == null ? undefined
              : draft.estimate_points === base.estimate_points && estimator ? `Set by ${estimator}`
              : draft.estimate_points !== base.estimate_points ? "Will be recorded as your estimate" : undefined}>
            <div className="flex items-center gap-2">
              <select id="editor-estimate" className={selectClass} value={draft.estimate_points ?? ""} onChange={(e) => set("estimate_points", e.target.value === "" ? null : Number(e.target.value))}>
                <option value="">Unestimated</option>
                {scale.map((n) => <option key={n} value={n}>{n}</option>)}
              </select>
              {draft.estimate_points != null && <Button type="button" variant="ghost" onClick={() => set("estimate_points", null)}>Clear</Button>}
            </div>
            {ticket.estimated_at && <p className="text-12 text-fg-secondary">Estimated <time dateTime={ticket.estimated_at}>{new Date(ticket.estimated_at).toLocaleString()}</time></p>}
            {ticket.estimate_rationale && <blockquote className="text-12 italic text-fg-secondary">{ticket.estimate_rationale}</blockquote>}
            {ticket.estimate && <p className="text-12 text-fg-secondary">Legacy note: {ticket.estimate}</p>}
            <FieldError id="err-estimate" message={err.estimate_points} />
          </Field>
          <Field label="Assignee" htmlFor="editor-assignee">
            <select id="editor-assignee" className={selectClass} value={draft.assignee_agent_id ?? ""} onChange={(e) => set("assignee_agent_id", e.target.value || null)}>
              <option value="">Unassigned</option>
              {agents.map((a) => <option key={a.id} value={a.id}>@{a.handle}</option>)}
            </select>
            <FieldError id="err-assignee" message={err.assignee_agent_id} />
          </Field>
          <Field label="Epic" htmlFor="editor-epic">
            <select id="editor-epic" className={selectClass} value={draft.epic_id ?? ""} onChange={(e) => set("epic_id", e.target.value || null)}>
              <option value="">No epic</option>
              {epics.map((e) => <option key={e.id} value={e.id}>{e.name}</option>)}
            </select>
            <FieldError id="err-epic" message={err.epic_id} />
          </Field>
        </div>
        <fieldset className="flex flex-col gap-2">
          <legend className="mb-1 text-13 font-medium text-fg-2">Acceptance criteria</legend>
          {criteria.length === 0 && <p className="text-12 text-fg-tertiary">No acceptance criteria yet.</p>}
          <ol className="flex flex-col gap-1.5">
            {criteria.map((c, i) => (
              <li key={c.uid} className="flex items-center gap-1.5">
                <input type="checkbox" aria-label={`Criterion ${i + 1} done`} checked={c.done}
                  onChange={(e) => setCriteria(criteria.map((x) => (x.uid === c.uid ? { ...x, done: e.target.checked } : x)))} />
                <TextInput aria-label={`Criterion ${i + 1} text`} value={c.text}
                  onChange={(e) => setCriteria(criteria.map((x) => (x.uid === c.uid ? { ...x, text: e.target.value } : x)))} />
                <Button type="button" variant="ghost" className={iconBtn} aria-label={`Move criterion ${i + 1} up`} disabled={i === 0} onClick={() => setCriteria(moveCriterion(criteria, i, -1))}>↑</Button>
                <Button type="button" variant="ghost" className={iconBtn} aria-label={`Move criterion ${i + 1} down`} disabled={i === criteria.length - 1} onClick={() => setCriteria(moveCriterion(criteria, i, 1))}>↓</Button>
                <Button type="button" variant="ghost" className={iconBtn} aria-label={`Delete criterion ${i + 1}`} onClick={() => setCriteria(criteria.filter((x) => x.uid !== c.uid))}>×</Button>
              </li>
            ))}
          </ol>
          <FieldError id="err-criteria" message={err.criteria} />
          <div><Button type="button" variant="ghost" onClick={() => setCriteria([...criteria, newCriterion()])}>+ Add criterion</Button></div>
        </fieldset>
      </form>
      <Footer ticketKey={ticket.key}>
        <Button type="button" variant="outline" onClick={onCancel}>Cancel</Button>
        <Button type="submit" form="ticket-editor-form" variant="primary" disabled={!patch || busy}>Save changes</Button>
      </Footer>
    </>
  );
}

function Footer({ ticketKey, children }: { ticketKey: string; children: ReactNode }) {
  return (
    <div className="flex items-center gap-2 border-t border-border-subtle px-[18px] py-3">
      <Link href={`/tickets/${encodeURIComponent(ticketKey)}`} className="mr-auto text-13 text-fg-secondary hover:text-fg">Open full ticket ↗</Link>
      {children}
    </div>
  );
}
