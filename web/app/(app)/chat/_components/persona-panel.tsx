"use client";

import { Dialog } from "radix-ui";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useEffect, useState } from "react";
import { useDeletePersona, usePersonas, useSetDefaultPersona, type Persona } from "@/lib/api/persona-hooks";
import { BUILTIN_COLOR, MAX_PERSONAS, deleteConsequence, isConflict, promotionCandidate, sortPersonas, toneLabel } from "@/lib/persona-ui";
import { cx } from "@/lib/cx";
import { PersonaAvatar, PersonaName } from "@/components/domain/persona-avatar";
import { Button, IconButton } from "@/components/ui/button";
import { Banner, Skeleton } from "@/components/ui/primitives";
import { PersonaEditor } from "./persona-editor";
import { CapabilityBadges } from "./persona";

const errorText = (e: unknown) => (e instanceof Error ? e.message : "Something went wrong. Please try again.");

/** The persona manager lives in the URL (?personas) like Memory; opening one closes the other. */
export function usePersonaPanel() {
  const router = useRouter();
  const pathname = usePathname();
  const search = useSearchParams();
  const go = (open: boolean) => {
    const q = new URLSearchParams(search.toString());
    if (open) {
      q.set("personas", "1");
      q.delete("memory");
      q.delete("canvas");
    } else q.delete("personas");
    router.replace(`${pathname}?${q.toString()}`, { scroll: false });
  };
  return { open: search.has("personas"), show: () => go(true), close: () => go(false) };
}

function DefaultBadge({ small }: { small?: boolean }) {
  return (
    <span className={cx("rounded-full bg-accent-soft px-2 text-accent", small ? "py-px text-[10px]" : "py-0.5 text-[11px]")}>Default</span>
  );
}

const link = "text-xs text-accent hover:underline disabled:opacity-50";

/**
 * Delete confirmation (mockup state 5b). Deleting the default asks who takes
 * over, preselecting the one the server would promote; the client sets that
 * default first so the choice holds.
 */
function DeleteDialog({ persona, all, onClose }: { persona: Persona; all: Persona[]; onClose: () => void }) {
  const del = useDeletePersona();
  const others = sortPersonas(all.filter((p) => p.id !== persona.id));
  const [next, setNext] = useState(() => promotionCandidate(all, persona.id)?.id);
  const confirm = () =>
    del
      .mutateAsync({ id: persona.id, revision: persona.revision, nextDefault: persona.is_default ? next : undefined })
      .then(onClose, () => undefined);
  return (
    <Dialog.Root open onOpenChange={(o) => !o && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-scrim" />
        <Dialog.Content className="fixed left-1/2 top-[18vh] z-50 flex w-[min(400px,calc(100%-32px))] -translate-x-1/2 flex-col gap-3 rounded-xl border border-border-float bg-surface-raised p-5 shadow-[var(--shadow-float)] outline-none">
          <Dialog.Title className="type-heading">Delete {persona.name}?</Dialog.Title>
          {persona.is_default ? (
            others.length ? (
              <fieldset className="flex flex-col gap-1">
                <legend className="mb-2 text-xs text-fg-secondary">{persona.name} is your default. Choose a new default:</legend>
                {others.map((p) => (
                  <label key={p.id} className={cx("flex cursor-pointer items-center gap-2.5 rounded-md px-2 py-1.5", next === p.id ? "bg-selected" : "hover:bg-hover")}>
                    <input type="radio" name="next-default" checked={next === p.id} onChange={() => setNext(p.id)} className="accent-[var(--accent)]" />
                    <PersonaAvatar name={p.name} color={p.color} src={p.avatar_url} size={18} />
                    <PersonaName name={p.name} color={p.color} className="text-[13px]" />
                  </label>
                ))}
              </fieldset>
            ) : (
              <p className="text-xs text-fg-secondary">{persona.name} is your default. The built-in Assistant becomes your default.</p>
            )
          ) : (
            <p className="text-xs text-fg-secondary">This can’t be undone.</p>
          )}
          <Dialog.Description className="text-xs text-fg-secondary">{deleteConsequence(persona.chat_count)}</Dialog.Description>
          {del.isError && (
            <p role="alert" className="text-xs text-pill-blocked-fg">
              {isConflict(del.error) ? `${persona.name} changed somewhere else. Close this and try again on the latest version.` : errorText(del.error)}
            </p>
          )}
          <div className="flex justify-end gap-2">
            <Dialog.Close asChild>
              <Button variant="secondary">Cancel</Button>
            </Dialog.Close>
            <Button
              variant="outline"
              className="border-pill-blocked-fg/40 bg-pill-blocked-bg font-semibold text-pill-blocked-fg hover:bg-pill-blocked-bg"
              disabled={del.isPending}
              onClick={confirm}
            >
              Delete {persona.name}
            </Button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

function Row({ p, onEdit, onDelete }: { p: Persona; onEdit: () => void; onDelete: () => void }) {
  const setDefault = useSetDefaultPersona();
  const excerpt = p.custom_instructions.trim().split("\n")[0];
  return (
    <li
      data-testid="persona-row"
      className={cx("flex gap-3 rounded-lg border border-border p-3", p.is_default && "bg-surface")}
    >
      <PersonaAvatar name={p.name} color={p.color} src={p.avatar_url} size={32} />
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <div className="flex items-center gap-2">
          <PersonaName name={p.name} color={p.color} className="truncate text-sm" />
          {p.is_default && <DefaultBadge />}
          <CapabilityBadges capabilities={p.capabilities} />
        </div>
        <p className="truncate text-xs text-fg-secondary">
          {toneLabel(p.tone)}
          {excerpt && <> · “{excerpt}”</>}
        </p>
        <div className="mt-1 flex flex-wrap items-center gap-3">
          {!p.is_default && (
            <button type="button" className={link} disabled={setDefault.isPending} onClick={() => setDefault.mutate(p.id)}>
              Set as default
            </button>
          )}
          <button type="button" className={link} onClick={onEdit} aria-label={`Edit ${p.name}`}>
            Edit
          </button>
          <button type="button" className="text-xs text-pill-blocked-fg hover:underline" onClick={onDelete} aria-label={`Delete ${p.name}`}>
            Delete
          </button>
        </div>
        {setDefault.isError && <p role="alert" className="text-xs text-pill-blocked-fg">{errorText(setDefault.error)}</p>}
      </div>
    </li>
  );
}

/** Shown when the person has no personas: the built-in Assistant is the default (mockup state 5a). */
function Empty({ onNew }: { onNew: () => void }) {
  return (
    <div className="flex flex-col gap-6">
      <div className="flex gap-3 rounded-lg border border-border bg-surface p-3">
        <PersonaAvatar name="Assistant" color={BUILTIN_COLOR} size={28} />
        <div className="flex flex-col gap-1">
          <div className="flex items-center gap-2">
            <PersonaName name="Assistant" color={BUILTIN_COLOR} className="text-[13px]" />
            <DefaultBadge small />
          </div>
          <p className="text-[11.5px] text-fg-tertiary">Built-in · Neutral tone · can’t be deleted</p>
        </div>
      </div>
      <div className="flex flex-col items-center gap-2 text-center">
        <p className="text-[13px] text-fg">No personas yet</p>
        <p className="max-w-[260px] text-xs text-fg-tertiary">Make one to give the Assistant a name, tone, color, avatar and instructions.</p>
        <Button variant="secondary" icon="plus" className="mt-1" onClick={onNew}>
          New persona
        </Button>
      </div>
    </div>
  );
}

/** The persona manager (mockup frame 2): list, set default, edit, delete, and the editor for new ones. */
export function PersonaPanel() {
  const panel = usePersonaPanel();
  const list = usePersonas();
  const [editing, setEditing] = useState<Persona | "new" | null>(null);
  const [deleting, setDeleting] = useState<Persona | null>(null);
  const all = list.data?.personas ?? [];
  const personas = sortPersonas(all);
  const full = all.length >= MAX_PERSONAS;
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && !editing && !deleting && panel.close();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });
  return (
    <aside
      aria-label="Personas"
      className="flex w-[440px] flex-none flex-col border-l border-border-strong bg-surface-sunken max-lg:w-[380px] max-md:absolute max-md:inset-0 max-md:z-20 max-md:w-full"
    >
      <div className="flex items-center gap-2 border-b border-border-subtle px-4 py-2.5">
        <h2 className="type-heading">Personas</h2>
        {list.data && <span className="text-xs text-fg-tertiary">{all.length}</span>}
        <span className="flex-1" />
        <Button variant="primary" icon="plus" disabled={!list.data || full} title={full ? `Up to ${MAX_PERSONAS} personas` : undefined} onClick={() => setEditing("new")}>
          New persona
        </Button>
        <IconButton icon="x" label="Close personas" onClick={panel.close} />
      </div>
      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto px-4 py-3">
        {list.isError ? (
          <Banner tone="blocked" title="Couldn’t load personas" actions={<Button variant="outline" onClick={() => list.refetch()}>Retry</Button>}>
            {errorText(list.error)}
          </Banner>
        ) : !list.data ? (
          <div aria-busy="true" className="flex flex-col gap-2">
            <Skeleton className="h-20" />
            <Skeleton className="h-20" />
          </div>
        ) : personas.length === 0 ? (
          <Empty onNew={() => setEditing("new")} />
        ) : (
          <>
            <ul className="flex flex-col gap-2">
              {personas.map((p) => (
                <Row key={p.id} p={p} onEdit={() => setEditing(p)} onDelete={() => setDeleting(p)} />
              ))}
            </ul>
            <p className="text-[11.5px] text-fg-tertiary">
              Default first, then A–Z. Up to {MAX_PERSONAS} personas.
            </p>
          </>
        )}
      </div>
      <div className="flex flex-col gap-1 border-t border-border-subtle px-4 py-3 text-[11.5px] text-fg-tertiary">
        <p>Personas shape your Assistant conversations only: name, tone, color, avatar.</p>
        <p>Agents working on tickets and epics never use a persona.</p>
        <p>Private to you.</p>
      </div>
      {editing && <PersonaEditor key={editing === "new" ? "new" : editing.id} persona={editing} onClose={() => setEditing(null)} />}
      {deleting && <DeleteDialog persona={deleting} all={all} onClose={() => setDeleting(null)} />}
    </aside>
  );
}
