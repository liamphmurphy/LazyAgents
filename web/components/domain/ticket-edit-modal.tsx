"use client";

import Link from "next/link";
import { Dialog } from "radix-ui";
import { useEffect, useState, type ReactNode } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, ApiError, unwrap, type Ticket, type TicketPatch } from "@/lib/api/client";
import { keys, useAddCriterion, useBootstrap, useDeleteCriterion, useTicket, useUpdateCriterion } from "@/lib/api/hooks";
import { BOARD_STATUSES } from "@/lib/board";
import { cx } from "@/lib/cx";
import { ago, absoluteTime } from "@/lib/format";
import {
  ESTIMATE_SCALE, FIELD_LABEL, changesOf, errorField, formOf, patchOf, reapply,
  type EditField, type EditForm,
} from "@/lib/ticket-edit";
import { Button, IconButton } from "@/components/ui/button";
import { Banner, Skeleton, TextArea, TextInput } from "@/components/ui/primitives";
import { AgentAvatar, ApiActorAvatar, UnassignedAvatar } from "./avatars";
import { PriorityGlyph, StatusGlyph } from "./glyphs";
import { Markdown } from "./markdown";
import { PRIORITY_NAME, STATUS_NAME } from "./meta";
import { AgentPicker, EpicMenu, PriorityMenu, StatusMenu } from "./pickers";

const MODAL_STATUSES = [...BOARD_STATUSES, "canceled" as const];

type Conflict = { mine: Partial<EditForm>; revision: number };
type SaveError = { field?: EditField; message: string };

/** Revision-checked PATCH. No optimistic write: a 409 must leave the cache on the server's version. */
function useSaveTicket(key: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (patch: TicketPatch) => unwrap(api.PATCH("/tickets/{key}", { params: { path: { key } }, body: patch })),
    onSuccess: (t) => qc.setQueryData(keys.ticket(key), t),
    onSettled: () => {
      qc.invalidateQueries({ queryKey: ["tickets", "list"] });
      qc.invalidateQueries({ queryKey: keys.activity(key) });
      qc.invalidateQueries({ queryKey: ["boards"] });
      qc.invalidateQueries({ queryKey: ["epics"] });
      qc.invalidateQueries({ queryKey: keys.bootstrap });
    },
  });
}

/** Click-to-edit modal opened from board cards (LAZYAGENTS-40 mockup). */
export function TicketEditModal({ ticketKey, onClose }: { ticketKey: string; onClose: () => void }) {
  const q = useTicket(ticketKey);
  const [dirty, setDirty] = useState(false);
  const close = () => {
    if (dirty && !window.confirm("Discard your unsaved changes?")) return;
    onClose();
  };
  return (
    <Dialog.Root open onOpenChange={(o) => !o && close()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-scrim" />
        <Dialog.Content
          aria-describedby={undefined}
          // Escape inside an inline criterion edit only cancels that edit.
          onEscapeKeyDown={(e) => { if ((e.target as HTMLElement | null)?.closest?.("[data-local-escape]")) e.preventDefault(); }}
          className="fixed left-1/2 top-[4vh] z-50 flex max-h-[92vh] w-[min(880px,calc(100%-32px))] -translate-x-1/2 flex-col overflow-hidden rounded-2xl border border-border-float bg-surface-raised shadow-[var(--shadow-float)] outline-none"
        >
          <div className="flex items-center gap-3 border-b border-border-subtle px-[18px] py-3">
            <Dialog.Title className="font-mono text-xs text-fg-tertiary">{ticketKey}</Dialog.Title>
            {q.data && <span className="text-xs text-fg-tertiary">rev {q.data.revision} · <Link href={`/tickets/${encodeURIComponent(ticketKey)}`} className="hover:text-fg hover:underline">Open full page ↗</Link></span>}
            <span className="ml-auto" />
            <IconButton icon="x" label="Close" onClick={close} />
          </div>
          {q.isPending ? (
            <div aria-busy="true" className="flex flex-col gap-2 p-[18px]"><Skeleton className="h-6 w-2/3" /><Skeleton className="h-32 w-full" /></div>
          ) : q.isError ? (
            <div className="p-[18px]"><Banner tone="blocked" title="Couldn't load the ticket">{q.error.message}</Banner></div>
          ) : (
            <EditBody key={q.data.key} latest={q.data} refetch={async () => (await q.refetch()).data} onDirty={setDirty} onClose={onClose} onCancel={close} />
          )}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

function EditBody({ latest, refetch, onDirty, onClose, onCancel }: {
  latest: Ticket; refetch: () => Promise<Ticket | undefined>; onDirty: (d: boolean) => void; onClose: () => void; onCancel: () => void;
}) {
  const boot = useBootstrap().data;
  const save = useSaveTicket(latest.key);
  // `base` is the server version the form started from; its revision goes into the PATCH.
  const [base, setBase] = useState(latest);
  const [form, setForm] = useState(() => formOf(latest));
  const [conflict, setConflict] = useState<Conflict>();
  const [error, setError] = useState<SaveError>();
  const [preview, setPreview] = useState(false);
  const changes = changesOf(formOf(base), form);
  const changed = Object.keys(changes) as EditField[];
  const isDirty = changed.length > 0;

  // Follow live updates (SSE, criteria refetches) while the person hasn't edited anything.
  // With unsaved edits the form stays on `base`, so a stale save gets the 409 flow.
  if (latest !== base && !isDirty && !conflict) {
    setBase(latest);
    setForm(formOf(latest));
  }
  useEffect(() => onDirty(isDirty), [isDirty, onDirty]);

  const set = <F extends EditField>(f: F, v: EditForm[F]) => {
    setForm((prev) => ({ ...prev, [f]: v }));
    if (error?.field === f) setError(undefined);
  };

  async function submit() {
    if (save.isPending) return;
    if (!isDirty) return onClose();
    setError(undefined);
    try {
      const t = await save.mutateAsync(patchOf(changes, base.revision));
      setConflict(undefined);
      setBase(t);
      setForm(formOf(t));
      onDirty(false);
      onClose();
    } catch (e) {
      if (!(e instanceof ApiError)) return setError({ message: e instanceof Error ? e.message : "Request failed. Try again." });
      if (e.status === 409) {
        const fresh = await refetch();
        if (fresh && fresh.revision !== base.revision) {
          // Stale revision: show the latest version and keep the person's edits aside.
          setConflict({ mine: changes, revision: fresh.revision });
          setBase(fresh);
          setForm(formOf(fresh));
          return;
        }
      }
      setError({ field: errorField(e.field), message: e.message });
    }
  }

  const agents = boot?.agents.filter((a) => a.kind === "main") ?? [];
  const epics = boot?.epics.filter((e) => e.project_id === base.project.id) ?? [];
  const assignee = form.assignee_agent_id === base.assignee?.id ? base.assignee : agents.find((a) => a.id === form.assignee_agent_id);
  const epicName = form.epic_id === base.epic?.id ? base.epic?.name : epics.find((e) => e.id === form.epic_id)?.name;
  const fieldError = (f: EditField) => error?.field === f ? <p role="alert" className="text-xs text-pill-blocked-fg">{error.message}</p> : null;
  const invalid = (f: EditField) => error?.field === f ? "!border-state-blocked" : "";

  return (
    <form
      className="flex min-h-0 flex-1 flex-col"
      onSubmit={(e) => { e.preventDefault(); void submit(); }}
      onKeyDown={(e) => { if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) { e.preventDefault(); void submit(); } }}
    >
      <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-[18px] py-4">
        {conflict && (
          <Banner
            title={`Someone else changed this ticket (now rev ${conflict.revision}).`}
            actions={<>
              <Button variant="outline" onClick={() => { setForm(reapply(formOf(base), conflict.mine)); setConflict(undefined); }}>Reapply mine</Button>
              <Button variant="outline" onClick={() => setConflict(undefined)}>Discard mine</Button>
            </>}
          >
            The form shows the latest version. Your {Object.keys(conflict.mine).length} unsaved edits ({(Object.keys(conflict.mine) as EditField[]).map((f) => FIELD_LABEL[f]).join(", ")}) are kept aside.
          </Banner>
        )}
        {error && !error.field && <Banner tone="blocked" title="Couldn't save">{error.message}</Banner>}
        <div className="grid gap-5 md:grid-cols-[1fr_240px]">
          <div className="flex min-w-0 flex-col gap-4">
            <Labeled label="Title" htmlFor="edit-title">
              <TextInput id="edit-title" value={form.title} onChange={(e) => set("title", e.target.value)} aria-invalid={error?.field === "title"} className={cx("!h-9 text-[15px] font-semibold", invalid("title"))} />
              {fieldError("title")}
            </Labeled>
            <Labeled label="Description (markdown)" htmlFor="edit-description" extra={
              <span className="flex gap-1 text-[11px]">
                <button type="button" aria-pressed={!preview} onClick={() => setPreview(false)} className={cx(!preview ? "text-fg" : "text-fg-tertiary")}>Write</button>·
                <button type="button" aria-pressed={preview} onClick={() => setPreview(true)} className={cx(preview ? "text-fg" : "text-fg-tertiary")}>Preview</button>
              </span>
            }>
              {preview
                ? <div className="min-h-40 rounded-md border border-border-subtle px-2.5 py-2"><Markdown>{form.description_md || "_Nothing to preview._"}</Markdown></div>
                : <TextArea id="edit-description" rows={8} value={form.description_md} onChange={(e) => set("description_md", e.target.value)} aria-invalid={error?.field === "description_md"} className={invalid("description_md")} />}
              {fieldError("description_md")}
            </Labeled>
            <CriteriaEditor t={latest} />
          </div>
          <div className="flex flex-col gap-3 md:border-l md:border-border-subtle md:pl-5">
            <Labeled label="Status">
              <StatusMenu value={form.status} statuses={MODAL_STATUSES} onChange={(s) => set("status", s)}>
                <PropButton invalid={invalid("status")}><StatusGlyph state={form.status} label={false} />{STATUS_NAME[form.status]}</PropButton>
              </StatusMenu>
              {fieldError("status")}
            </Labeled>
            <Labeled label="Priority">
              <PriorityMenu value={form.priority} onChange={(p) => set("priority", p)}>
                <PropButton invalid={invalid("priority")}><PriorityGlyph priority={form.priority} label={false} />{PRIORITY_NAME[form.priority]}</PropButton>
              </PriorityMenu>
              {fieldError("priority")}
            </Labeled>
            <Labeled label="Assignee">
              <AgentPicker agents={agents} value={form.assignee_agent_id} onChange={(id) => set("assignee_agent_id", id)}>
                <PropButton invalid={invalid("assignee_agent_id")}>
                  {assignee ? <><AgentAvatar name={assignee.name} colorIndex={assignee.color_index} label={false} />@{assignee.handle}</> : <><UnassignedAvatar />Unassigned</>}
                </PropButton>
              </AgentPicker>
              {fieldError("assignee_agent_id")}
            </Labeled>
            <Labeled label="Epic">
              <EpicMenu epics={epics} value={form.epic_id} onChange={(id) => set("epic_id", id)}>
                <PropButton invalid={invalid("epic_id")}>{epicName ?? <span className="text-fg-tertiary">No epic</span>}</PropButton>
              </EpicMenu>
              {fieldError("epic_id")}
            </Labeled>
            <EstimatePicker t={base} value={form.estimate_points} onChange={(v) => set("estimate_points", v)} invalid={invalid("estimate_points")} error={fieldError("estimate_points")} />
          </div>
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-3 border-t border-track bg-surface-sunken px-[18px] py-3">
        <span className="mr-auto text-xs text-fg-tertiary">
          {isDirty ? `${changed.length} unsaved ${changed.length === 1 ? "change" : "changes"} · ` : ""}⌘↵ to save · Esc to close
        </span>
        <Button variant="ghost" onClick={onCancel}>Cancel</Button>
        <Button type="submit" variant="primary" disabled={save.isPending || !!conflict}>{save.isPending ? "Saving…" : "Save"}</Button>
      </div>
    </form>
  );
}

function Labeled({ label, htmlFor, extra, children }: { label: string; htmlFor?: string; extra?: ReactNode; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-center justify-between">
        <label htmlFor={htmlFor} className="text-[11px] font-medium uppercase tracking-wide text-fg-tertiary">{label}</label>
        {extra}
      </div>
      {children}
    </div>
  );
}

function PropButton({ invalid, children, ...rest }: { invalid?: string; children: ReactNode } & React.ComponentProps<"button">) {
  return (
    <button type="button" {...rest} className={cx("flex h-[30px] w-full items-center gap-2 rounded-md border border-border-subtle bg-surface px-2.5 text-left text-[13px] hover:bg-hover", invalid)}>
      {children}<span aria-hidden className="ml-auto text-fg-tertiary">▾</span>
    </button>
  );
}

function EstimatePicker({ t, value, onChange, invalid, error }: {
  t: Ticket; value: number | null; onChange: (v: number | null) => void; invalid: string; error: ReactNode;
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <span id="estimate-label" className="text-[11px] font-medium uppercase tracking-wide text-fg-tertiary">Estimate (points)</span>
      <div role="radiogroup" aria-labelledby="estimate-label" className={cx("flex gap-1 rounded-md", invalid && "outline outline-1 outline-state-blocked")}>
        {ESTIMATE_SCALE.map((p) => (
          <button key={p} type="button" role="radio" aria-checked={value === p} onClick={() => onChange(p)}
            className={cx("h-7 flex-1 rounded-md border text-xs", value === p ? "border-accent bg-accent text-on-accent" : "border-border-subtle bg-surface hover:bg-hover")}>
            {p}
          </button>
        ))}
      </div>
      {error}
      {value != null && <button type="button" onClick={() => onChange(null)} className="self-start text-xs text-fg-tertiary hover:text-fg">Clear estimate</button>}
      {(t.estimated_by || t.estimate) && (
        <div className="flex flex-col gap-1.5 rounded-md bg-surface-sunken p-2.5 text-xs">
          {t.estimated_by && (
            <span className="flex items-center gap-1.5 text-fg-2">
              Estimated by <ApiActorAvatar actor={t.estimated_by} /> {t.estimated_by.type === "agent" ? "@" : ""}{t.estimated_by.name}
              {t.estimated_at && <time dateTime={t.estimated_at} title={absoluteTime(t.estimated_at)} className="text-fg-tertiary">· {ago(t.estimated_at)}</time>}
            </span>
          )}
          {t.estimate_rationale && <blockquote className="italic text-fg-secondary">“{t.estimate_rationale}”</blockquote>}
          {t.estimate && <span className="text-fg-tertiary">Legacy note: {t.estimate}</span>}
        </div>
      )}
      {t.estimated_by?.type === "agent" && <p className="text-[11px] text-fg-tertiary">Saving your own points replaces the agent&apos;s estimate and its rationale.</p>}
    </div>
  );
}

/** Criteria save instantly through their own endpoints; they don't bump the ticket revision. */
function CriteriaEditor({ t }: { t: Ticket }) {
  const add = useAddCriterion(t.key);
  const update = useUpdateCriterion(t.key);
  const remove = useDeleteCriterion(t.key);
  const [draft, setDraft] = useState("");
  const [editing, setEditing] = useState<{ id: string; text: string }>();
  const done = t.criteria.filter((c) => c.done).length;
  const failure = add.error ?? update.error ?? remove.error;

  function commitEdit() {
    if (!editing) return;
    const text = editing.text.trim();
    const current = t.criteria.find((c) => c.id === editing.id);
    if (text && current && text !== current.text) update.mutate({ id: editing.id, text });
    setEditing(undefined);
  }

  return (
    <section aria-label="Acceptance criteria" className="flex flex-col gap-1.5">
      <h3 className="text-[11px] font-medium uppercase tracking-wide text-fg-tertiary">
        Acceptance criteria · {done} of {t.criteria.length} <span className="normal-case tracking-normal">· saved instantly, no revision bump</span>
      </h3>
      <ul className="flex flex-col">
        {t.criteria.map((c) => (
          <li key={c.id} className="group flex min-h-8 items-center gap-2.5">
            <input type="checkbox" aria-label={`Criterion ${c.position} met`} checked={c.done}
              onChange={(e) => update.mutate({ id: c.id, done: e.target.checked })} className="size-3.5 flex-none accent-[var(--accent)]" />
            {editing?.id === c.id ? (
              <input autoFocus data-local-escape aria-label={`Edit criterion ${c.position}`} value={editing.text}
                onChange={(e) => setEditing({ id: c.id, text: e.target.value })}
                onBlur={commitEdit}
                onKeyDown={(e) => {
                  if (e.key === "Enter" && !e.metaKey && !e.ctrlKey) { e.preventDefault(); commitEdit(); }
                  if (e.key === "Escape") setEditing(undefined);
                }}
                className="h-7 flex-1 rounded border border-accent bg-surface px-2 text-[13px] outline-none" />
            ) : (
              <span className={cx("flex-1 text-[13px]", c.done ? "text-fg-tertiary line-through" : "text-fg-2")}>{c.position}. {c.text}</span>
            )}
            {editing?.id !== c.id && <>
              <IconButton icon="edit" label={`Edit criterion ${c.position}`} size={24} onClick={() => setEditing({ id: c.id, text: c.text })} className="opacity-0 focus-visible:opacity-100 group-hover:opacity-100" />
              <IconButton icon="x" label={`Remove criterion ${c.position}`} size={24} onClick={() => remove.mutate(c.id)} className="opacity-0 focus-visible:opacity-100 group-hover:opacity-100" />
            </>}
          </li>
        ))}
      </ul>
      <div className="flex items-center gap-2.5">
        <span aria-hidden className="size-3.5 flex-none rounded-[3px] border border-dashed border-border-strong" />
        <input aria-label="Add criterion" value={draft} onChange={(e) => setDraft(e.target.value)} placeholder="+ Add criterion…"
          onKeyDown={(e) => {
            if (e.key !== "Enter" || e.metaKey || e.ctrlKey) return;
            e.preventDefault();
            if (draft.trim()) add.mutate(draft.trim(), { onSuccess: () => setDraft("") });
          }}
          className="h-8 flex-1 bg-transparent text-[13px] text-fg outline-none placeholder:text-fg-tertiary" />
      </div>
      {failure && <p role="alert" className="text-xs text-pill-blocked-fg">{failure.message}</p>}
    </section>
  );
}
