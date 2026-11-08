"use client";

import { Dialog } from "radix-ui";
import { useState } from "react";
import type { Schemas } from "@/lib/api/client";
import { useDeletePersona, usePersonas, useSetDefaultPersona, type Persona } from "@/lib/api/persona-hooks";
import { MAX_PERSONAS, deleteConsequence, isConflict, promotionCandidate, sortPersonas, toneLabel } from "@/lib/persona-ui";
import { cx } from "@/lib/cx";
import { Content, TopBar } from "@/components/shell/topbar";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { Banner, Skeleton } from "@/components/ui/primitives";
import { PersonaEditor } from "../chat/_components/persona-editor";
import { BUILTIN, CAPABILITIES, PersonaAvatar, PersonaName } from "../chat/_components/persona";

const errorText = (e: unknown) => (e instanceof Error ? e.message : "Something went wrong. Please try again.");

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
                    <PersonaAvatar persona={p} size={18} />
                    <PersonaName persona={p} className="text-[13px]" />
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

const card = "relative flex min-w-0 flex-col items-center gap-3 rounded-xl border bg-surface px-5 pb-5 pt-8 text-center";

/** The large picture every profile card leads with, ringed in the persona's color. */
function Portrait({ persona }: { persona: { name: string; color: string; avatar_url?: string } }) {
  return (
    <span className="flex rounded-full p-1" style={{ boxShadow: `inset 0 0 0 2px color-mix(in srgb, ${persona.color} 45%, transparent)` }}>
      <PersonaAvatar persona={persona} size={104} />
    </span>
  );
}

/** What the person has allowed this persona to do, spelled out. */
function Permissions({ capabilities }: { capabilities?: Schemas["PersonaCapabilities"] }) {
  const granted = CAPABILITIES.filter((c) => capabilities?.[c.key]);
  if (!granted.length) return <p className="text-[11.5px] text-fg-tertiary">No extra permissions</p>;
  return (
    <ul aria-label="Permissions" data-testid="persona-permissions" className="flex flex-wrap justify-center gap-1">
      {granted.map((c) => (
        <li key={c.key} title={c.title} className="inline-flex items-center gap-1 rounded-full bg-selected px-2 py-0.5 text-[11px] text-fg-secondary">
          <Icon name={c.icon} size={12} />
          {c.label}
        </li>
      ))}
    </ul>
  );
}

/** One profile: click the picture to edit it. */
function ProfileCard({ p, onEdit, onDelete }: { p: Persona; onEdit: () => void; onDelete: () => void }) {
  const setDefault = useSetDefaultPersona();
  return (
    <li data-testid="persona-row" className={cx(card, "group", p.is_default ? "border-accent" : "border-border hover:border-border-strong")}>
      {p.is_default && (
        <span className="absolute left-3 top-3">
          <DefaultBadge />
        </span>
      )}
      <button
        type="button"
        onClick={onEdit}
        aria-label={`Edit ${p.name}`}
        className="flex max-w-full flex-col items-center gap-3 rounded-lg outline-none focus-visible:ring-2 focus-visible:ring-accent"
      >
        <span className="relative">
          <Portrait persona={p} />
          <span
            aria-hidden
            className="absolute bottom-1 right-1 grid size-7 place-items-center rounded-full border border-border-strong bg-surface-raised text-fg-secondary opacity-0 transition-opacity group-focus-within:opacity-100 group-hover:opacity-100"
          >
            <Icon name="edit" size={13} />
          </span>
        </span>
        <PersonaName persona={p} className="max-w-full truncate text-[15px]" />
      </button>
      <p className="-mt-2 text-xs text-fg-tertiary">{toneLabel(p.tone)} tone</p>
      <Permissions capabilities={p.capabilities} />
      <div className="mt-auto flex flex-wrap items-center justify-center gap-3 pt-1">
        {!p.is_default && (
          <button type="button" className={link} disabled={setDefault.isPending} onClick={() => setDefault.mutate(p.id)}>
            Set as default
          </button>
        )}
        <button type="button" className="text-xs text-pill-blocked-fg hover:underline" onClick={onDelete} aria-label={`Delete ${p.name}`}>
          Delete
        </button>
      </div>
      {setDefault.isError && <p role="alert" className="text-xs text-pill-blocked-fg">{errorText(setDefault.error)}</p>}
    </li>
  );
}

/** The "add a profile" tile that closes the grid. */
function AddTile({ onNew }: { onNew: () => void }) {
  return (
    <li className="flex">
      <button
        type="button"
        onClick={onNew}
        className="flex min-h-[248px] flex-1 flex-col items-center justify-center gap-3 rounded-xl border border-dashed border-border-strong text-fg-secondary outline-none hover:bg-hover hover:text-fg focus-visible:ring-2 focus-visible:ring-accent"
      >
        <span className="grid size-16 place-items-center rounded-full bg-selected">
          <Icon name="plus" size={24} />
        </span>
        <span className="text-[13px]">New persona</span>
      </button>
    </li>
  );
}

/** Stands in when the person has no personas: the built-in Assistant is the default (mockup state 5a). */
function BuiltinCard() {
  return (
    <li className={cx(card, "border-accent")}>
      <span className="absolute left-3 top-3">
        <DefaultBadge />
      </span>
      <Portrait persona={BUILTIN} />
      <PersonaName persona={BUILTIN} className="text-[15px]" />
      <p className="-mt-2 text-xs text-fg-tertiary">Built-in · Neutral tone · can’t be deleted</p>
    </li>
  );
}

const grid = "grid grid-cols-[repeat(auto-fill,minmax(220px,1fr))] gap-4";

/** The persona manager: a profile-style grid to set the default, edit, delete and add personas. */
export function PersonasScreen() {
  const list = usePersonas();
  const [editing, setEditing] = useState<Persona | "new" | null>(null);
  const [deleting, setDeleting] = useState<Persona | null>(null);
  const all = list.data?.personas ?? [];
  const personas = sortPersonas(all);
  return (
    <section aria-label="Personas">
      <TopBar
        crumbs={[{ label: "Assistant", href: "/chat" }, { label: "Personas" }]}
        actions={list.data && <span className="text-xs text-fg-tertiary">{all.length} of {MAX_PERSONAS}</span>}
      />
      <Content>
        {list.isError ? (
          <Banner tone="blocked" title="Couldn’t load personas" actions={<Button variant="outline" onClick={() => list.refetch()}>Retry</Button>}>
            {errorText(list.error)}
          </Banner>
        ) : !list.data ? (
          <div aria-busy="true" className={grid}>
            <Skeleton className="h-[248px]" />
            <Skeleton className="h-[248px]" />
            <Skeleton className="h-[248px]" />
          </div>
        ) : (
          <>
            <ul className={grid}>
              {personas.length === 0 && <BuiltinCard />}
              {personas.map((p) => (
                <ProfileCard key={p.id} p={p} onEdit={() => setEditing(p)} onDelete={() => setDeleting(p)} />
              ))}
              {all.length < MAX_PERSONAS && <AddTile onNew={() => setEditing("new")} />}
            </ul>
            {personas.length === 0 && (
              <p className="mt-6 text-center text-[13px] text-fg-secondary">
                No personas yet. Make one to give the Assistant a name, tone, color, avatar and instructions.
              </p>
            )}
          </>
        )}
        <p className="mt-8 text-center text-[11.5px] text-fg-tertiary">
          Personas shape your Assistant conversations only. Agents working on tickets and epics never use one. Private to you.
        </p>
      </Content>
      {editing && <PersonaEditor key={editing === "new" ? "new" : editing.id} persona={editing} onClose={() => setEditing(null)} />}
      {deleting && <DeleteDialog persona={deleting} all={all} onClose={() => setDeleting(null)} />}
    </section>
  );
}
