"use client";

import Link from "next/link";
import { useState, type FormEvent } from "react";
import type { Schemas } from "@/lib/api/client";
import { useDeleteMemory } from "@/lib/api/hooks";
import { useCreateMemory, useMoments, usePersonaLookup } from "@/lib/api/persona-memory-hooks";
import type { Persona } from "@/lib/api/persona-hooks";
import { cx } from "@/lib/cx";
import { Button, IconButton } from "@/components/ui/button";
import { Banner, EmptyState, Skeleton, TextArea, TextInput } from "@/components/ui/primitives";
import { undoable } from "@/components/ui/toast";

type Memory = Schemas["Memory"];

const day = (iso: string) => new Date(iso).toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" });
const errorText = (e: unknown) => (e instanceof Error ? e.message : "Something went wrong. Please try again.");

/** One moment, pinned like a note on the fridge in its persona's colour. */
function MomentCard({ m, persona }: { m: Memory; persona?: Persona }) {
  const del = useDeleteMemory();
  const color = persona?.color ?? "var(--border-strong)";
  // Deleted but still undoable: hidden here until the toast goes and the delete runs.
  const [hidden, setHidden] = useState(false);
  if (hidden) return null;
  return (
    <li
      data-testid="fridge-moment"
      className="flex min-w-0 flex-col gap-2 rounded-lg border border-border bg-surface p-3"
      style={{ borderTopColor: color, borderTopWidth: 3 }}
    >
      <div className="flex items-start gap-2">
        <span className="min-w-0 flex-1 text-13 font-medium text-fg">{m.title}</span>
        <IconButton
          icon="trash"
          label={`Delete ${m.title}`}
          disabled={del.isPending}
          onClick={() => undoable({
            title: "Moment deleted",
            description: `${persona?.name ?? "The persona"} will no longer remember it.`,
            failed: "Couldn't delete the moment",
            hide: () => setHidden(true),
            restore: () => setHidden(false),
            commit: () => del.mutateAsync(m.id),
          })}
        />
      </div>
      {m.summary && <p className="text-13 text-fg-secondary">{m.summary}</p>}
      {m.body && m.body !== m.summary && <p className="whitespace-pre-wrap text-12 text-fg-secondary">{m.body}</p>}
      <p className="mt-auto flex flex-wrap items-center gap-1 text-12 text-fg-tertiary">
        <span aria-hidden className="size-2 rounded-full" style={{ background: color }} />
        <span>{persona?.name ?? "A deleted persona"}</span>
        <span>· {day(m.created_at)}</span>
        {m.source_chat_id && (
          <>
            {" · "}
            <Link href={`/chat/${m.source_chat_id}`} className="text-accent hover:underline">
              Open chat
            </Link>
          </>
        )}
      </p>
    </li>
  );
}

/** Pin a moment by hand to one persona's own memory. */
function AddMoment({ personas, initial, onDone }: { personas: Persona[]; initial: string; onDone: () => void }) {
  const create = useCreateMemory();
  const [persona, setPersona] = useState(initial || personas[0]?.id || "");
  const [title, setTitle] = useState("");
  const [summary, setSummary] = useState("");
  function submit(e: FormEvent) {
    e.preventDefault();
    if (!title.trim() || !persona) return;
    create
      .mutateAsync({ title: title.trim(), summary: summary.trim(), type: "moment", persona_id: persona })
      .then(onDone, () => undefined);
  }
  return (
    <form onSubmit={submit} aria-label="Add a moment" className="flex flex-col gap-2 rounded-lg border border-accent bg-surface p-3">
      <div className="flex gap-2">
        <select
          aria-label="Persona"
          value={persona}
          onChange={(e) => setPersona(e.target.value)}
          className="h-[30px] w-40 flex-none rounded-md border border-border-strong bg-surface px-2 text-13 text-fg focus:border-accent focus:outline-none"
        >
          {personas.map((p) => (
            <option key={p.id} value={p.id}>
              {p.name}
            </option>
          ))}
        </select>
        <TextInput aria-label="Title" placeholder="What happened, in a few words" value={title} maxLength={200} required autoFocus className="min-w-0 flex-1" onChange={(e) => setTitle(e.target.value)} />
      </div>
      <TextArea aria-label="Summary" rows={2} placeholder="One line to remember it by" value={summary} maxLength={300} onChange={(e) => setSummary(e.target.value)} />
      {create.isError && (
        <p role="alert" className="text-12 text-pill-blocked-fg">
          {errorText(create.error)}
        </p>
      )}
      <div className="flex gap-2">
        <Button type="submit" variant="primary" disabled={create.isPending || !title.trim() || !persona}>
          Pin it
        </Button>
        <Button variant="outline" onClick={onDone}>
          Cancel
        </Button>
      </div>
    </form>
  );
}

const grid = "grid grid-cols-[repeat(auto-fill,minmax(220px,1fr))] gap-3";

/** The Library's Fridge: the moments each persona chose to keep from your chats, newest first. */
export function Fridge() {
  const lookup = usePersonaLookup();
  const personas = [...lookup.values()];
  const [filter, setFilter] = useState("");
  const [adding, setAdding] = useState(false);
  const moments = useMoments(filter);
  const items = moments.data?.pages.flatMap((p) => p.memories) ?? [];
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-1">
        {[{ id: "", name: "Everyone", color: "" }, ...personas].map((p) => (
          <button
            key={p.id || "all"}
            type="button"
            aria-pressed={filter === p.id}
            onClick={() => setFilter(p.id)}
            className={cx(
              "inline-flex items-center gap-1.5 rounded-md px-2.5 py-1 text-12",
              filter === p.id ? "bg-selected text-fg" : "text-fg-secondary hover:bg-hover",
            )}
          >
            {p.color && <span aria-hidden className="size-2 rounded-full" style={{ background: p.color }} />}
            {p.name}
          </button>
        ))}
        <span className="flex-1" />
        {personas.length > 0 && !adding && (
          <Button icon="plus" onClick={() => setAdding(true)}>
            Add a moment
          </Button>
        )}
      </div>
      {adding && <AddMoment personas={personas} initial={filter} onDone={() => setAdding(false)} />}
      {moments.isError ? (
        <Banner tone="blocked" title="Couldn't load the Fridge" actions={<Button onClick={() => moments.refetch()}>Retry</Button>}>
          {moments.error.message}
        </Banner>
      ) : moments.isPending ? (
        <ul aria-busy="true" aria-label="Loading moments" className={grid}>
          {[0, 1, 2].map((i) => (
            <li key={i}>
              <Skeleton className="h-28 w-full rounded-lg" />
            </li>
          ))}
        </ul>
      ) : items.length === 0 ? (
        <EmptyState icon="bookmark" title="Nothing on the Fridge yet">
          When a persona wants to keep a moment you shared, it pins it here. You can add one yourself too.
        </EmptyState>
      ) : (
        <>
          <ul className={grid}>
            {items.map((m) => (
              <MomentCard key={m.id} m={m} persona={m.persona_id ? lookup.get(m.persona_id) : undefined} />
            ))}
          </ul>
          {moments.hasNextPage && (
            <div className="flex justify-center">
              <Button onClick={() => moments.fetchNextPage()} disabled={moments.isFetchingNextPage}>
                {moments.isFetchingNextPage ? "Loading…" : "Show more"}
              </Button>
            </div>
          )}
        </>
      )}
    </div>
  );
}
