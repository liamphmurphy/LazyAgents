"use client";

import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useEffect, useState, type FormEvent } from "react";
import type { Schemas } from "@/lib/api/client";
import {
  exportMemories,
  useDeleteAllMemories,
  useDeleteMemory,
  useMemories,
  useMemory,
  useUpdateMemory,
  type MemoryType,
} from "@/lib/api/hooks";
import { MEMORY_TYPES, memoriesMarkdown, parseTags, typeLabel } from "@/lib/memory-ui";
import { cx } from "@/lib/cx";
import { Button, IconButton } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { Banner, Skeleton, TextArea, TextInput } from "@/components/ui/primitives";

type Memory = Schemas["Memory"];
const errorText = (e: unknown) => (e instanceof Error ? e.message : "Something went wrong. Please try again.");

/** The memory manager lives in the URL (?memory, or ?memory=<id> to open one entry) so "View" can deep-link it. */
export function useMemoryPanel() {
  const router = useRouter();
  const pathname = usePathname();
  const search = useSearchParams();
  const raw = search.get("memory");
  const go = (value: string | null) => {
    const q = new URLSearchParams(search.toString());
    if (value === null) q.delete("memory");
    else q.set("memory", value);
    router.replace(`${pathname}?${q.toString()}`, { scroll: false });
  };
  return {
    open: raw !== null,
    focus: raw && raw !== "1" ? raw : undefined,
    show: (id?: string) => go(id ?? "1"),
    close: () => go(null),
  };
}

/** Header entry point: "Memory" with the entry count; toggles the panel. */
export function MemoryButton() {
  const panel = useMemoryPanel();
  const count = useMemories();
  const total = count.data?.total;
  return (
    <button
      type="button"
      aria-pressed={panel.open}
      aria-label={total === undefined ? "Memory" : `Memory, ${total} entries`}
      onClick={() => (panel.open ? panel.close() : panel.show())}
      className={cx(
        "inline-flex h-[30px] items-center gap-1.5 rounded-md border px-3 text-[13px] transition-colors",
        panel.open ? "border-accent bg-selected text-fg" : "border-btn-secondary-border bg-btn-secondary text-fg hover:bg-hover",
      )}
    >
      <Icon name="bookmark" size={14} className={panel.open ? "text-accent" : undefined} />
      Memory
      {total !== undefined && <span className="text-xs text-fg-tertiary">{total}</span>}
    </button>
  );
}

const shortDate = (iso: string) => new Date(iso).toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" });

/** Where an entry came from: its conversation (linked while it exists), or the raw source. */
function SourceText({ m }: { m: Memory }) {
  if (m.source_chat_id)
    return (
      <Link href={`/chat/${m.source_chat_id}`} className="text-accent hover:underline">
        from a conversation ↗
      </Link>
    );
  if (m.source.startsWith("chat ")) return <>from a deleted conversation</>;
  if (!m.source || m.source === "manual") return <>added by you</>;
  return <>from {m.source}</>;
}

function Meta({ m }: { m: Memory }) {
  return (
    <p className="text-[11.5px] text-fg-tertiary">
      {typeLabel(m.type)} · <SourceText m={m} /> · saved {shortDate(m.created_at)}
      {m.updated_at !== m.created_at && ` · updated ${shortDate(m.updated_at)}`}
    </p>
  );
}

function Tags({ tags }: { tags: string[] }) {
  if (!tags.length) return null;
  return (
    <ul className="flex flex-wrap gap-1" aria-label="Tags">
      {tags.map((t) => (
        <li key={t} className="rounded-full bg-selected px-2 py-px text-[11px] text-fg-secondary">
          {t}
        </li>
      ))}
    </ul>
  );
}

const label = "text-[11px] font-medium uppercase tracking-wide text-fg-tertiary";

function Editor({ m, onDone, onDeleted }: { m: Memory; onDone: () => void; onDeleted: () => void }) {
  const update = useUpdateMemory();
  const del = useDeleteMemory();
  const [title, setTitle] = useState(m.title);
  const [summary, setSummary] = useState(m.summary);
  const [body, setBody] = useState(m.body);
  const [type, setType] = useState<string>(m.type);
  const [tags, setTags] = useState(m.tags.join(", "));
  const id = `memory-${m.id}`;

  function save(e: FormEvent) {
    e.preventDefault();
    if (!title.trim()) return;
    update
      .mutateAsync({ id: m.id, patch: { title: title.trim(), summary: summary.trim(), body, type: type as MemoryType, tags: parseTags(tags) } })
      .then(onDone, () => undefined);
  }
  const err = update.error ?? del.error;
  return (
    <form onSubmit={save} aria-label={`Edit ${m.title}`} className="flex flex-col gap-2 rounded-lg border border-accent bg-surface p-3">
      <label htmlFor={`${id}-title`} className={label}>Title</label>
      <TextInput id={`${id}-title`} value={title} maxLength={200} required autoFocus onChange={(e) => setTitle(e.target.value)} />
      <label htmlFor={`${id}-summary`} className={label}>Summary</label>
      <TextArea id={`${id}-summary`} rows={2} value={summary} maxLength={300} onChange={(e) => setSummary(e.target.value)} />
      <label htmlFor={`${id}-body`} className={label}>Details</label>
      <TextArea id={`${id}-body`} rows={3} value={body} maxLength={20000} onChange={(e) => setBody(e.target.value)} />
      <div className="flex gap-2">
        <div className="flex w-36 flex-none flex-col gap-1">
          <label htmlFor={`${id}-type`} className={label}>Type</label>
          <select
            id={`${id}-type`}
            value={type}
            onChange={(e) => setType(e.target.value)}
            className="h-[30px] rounded-md border border-border-strong bg-surface px-2 text-[13px] text-fg focus:border-accent focus:outline-none"
          >
            {MEMORY_TYPES.map((t) => (
              <option key={t} value={t}>
                {typeLabel(t)}
              </option>
            ))}
          </select>
        </div>
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <label htmlFor={`${id}-tags`} className={label}>Tags</label>
          <TextInput id={`${id}-tags`} value={tags} placeholder="comma, separated" onChange={(e) => setTags(e.target.value)} />
        </div>
      </div>
      <Meta m={m} />
      {err && (
        <p role="alert" className="text-xs text-pill-blocked-fg">
          {errorText(err)}
        </p>
      )}
      <div className="flex items-center gap-2">
        <Button type="submit" variant="primary" disabled={update.isPending || !title.trim()}>
          Save
        </Button>
        <Button variant="outline" onClick={onDone}>
          Cancel
        </Button>
        <span className="flex-1" />
        <Button variant="ghost" className="text-pill-blocked-fg" disabled={del.isPending} onClick={() => del.mutateAsync(m.id).then(onDeleted, () => undefined)}>
          Delete
        </Button>
      </div>
    </form>
  );
}

function Row({ m, editing, onEdit, onDone }: { m: Memory; editing: boolean; onEdit: () => void; onDone: () => void }) {
  const del = useDeleteMemory();
  if (editing) return <li><Editor m={m} onDone={onDone} onDeleted={onDone} /></li>;
  return (
    <li className="group flex gap-2 rounded-lg border border-border p-3 hover:bg-hover" data-testid="memory-row">
      <button type="button" onClick={onEdit} className="flex min-w-0 flex-1 flex-col items-start gap-1 text-left">
        <span className="text-[13px] text-fg">{m.title}</span>
        {m.summary && <span className="text-xs text-fg-secondary">{m.summary}</span>}
        <Tags tags={m.tags} />
        <Meta m={m} />
      </button>
      <div className="flex flex-none flex-col gap-1">
        <IconButton icon="edit" label={`Edit ${m.title}`} onClick={onEdit} />
        <IconButton icon="trash" label={`Delete ${m.title}`} disabled={del.isPending} onClick={() => del.mutate(m.id)} />
      </div>
    </li>
  );
}

/** One page of 50; the last loaded page offers the next. Search returns only the top 50. */
function Page({
  q,
  type,
  offset,
  hide,
  editing,
  setEditing,
  onMore,
  last,
}: {
  q: string;
  type?: MemoryType;
  offset: number;
  hide?: string;
  editing?: string;
  setEditing: (id?: string) => void;
  onMore: () => void;
  last: boolean;
}) {
  const list = useMemories(q, type, offset);
  if (list.isError) return <Banner tone="blocked" title="Couldn’t load memories">{errorText(list.error)}</Banner>;
  if (!list.data)
    return (
      <div aria-busy="true" className="flex flex-col gap-2">
        <Skeleton className="h-16" />
        <Skeleton className="h-16" />
      </div>
    );
  const rows = list.data.memories.filter((m) => m.id !== hide);
  return (
    <>
      {offset === 0 && rows.length === 0 && (
        <p className="px-1 py-6 text-center text-[13px] text-fg-secondary">
          {q || type ? "No memories match." : "Nothing remembered yet. The assistant saves useful things as you talk and tells you when it does."}
        </p>
      )}
      <ul className="flex flex-col gap-2">
        {rows.map((m) => (
          <Row key={m.id} m={m} editing={editing === m.id} onEdit={() => setEditing(m.id)} onDone={() => setEditing(undefined)} />
        ))}
      </ul>
      {last && list.data.has_more && !q && (
        <Button variant="outline" className="self-center" onClick={onMore}>
          Load more
        </Button>
      )}
    </>
  );
}

function download(name: string, type: string, text: string) {
  const url = URL.createObjectURL(new Blob([text], { type }));
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

function ExportMenu() {
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  async function run(format: "json" | "md") {
    setBusy(true);
    setErr(null);
    try {
      const data = await exportMemories();
      const day = data.exported_at.slice(0, 10);
      if (format === "json") download(`memory-${day}.json`, "application/json", JSON.stringify(data, null, 2));
      else download(`memory-${day}.md`, "text/markdown", memoriesMarkdown(data.memories, data.exported_at));
    } catch (e) {
      setErr(errorText(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <details className="relative">
      <summary className="inline-flex h-[30px] cursor-pointer list-none items-center gap-1.5 rounded-md border border-btn-secondary-border bg-btn-secondary px-3 text-[12.5px] text-fg hover:bg-hover">
        <Icon name="download" size={14} />
        Export
        <Icon name="chev" size={12} />
      </summary>
      <div className="absolute right-0 z-10 mt-1 flex w-44 flex-col rounded-md border border-border-strong bg-surface p-1 shadow-lg">
        <button type="button" disabled={busy} className="rounded px-2 py-1.5 text-left text-[13px] hover:bg-hover" onClick={() => run("json")}>
          JSON (all entries)
        </button>
        <button type="button" disabled={busy} className="rounded px-2 py-1.5 text-left text-[13px] hover:bg-hover" onClick={() => run("md")}>
          Markdown (all entries)
        </button>
        {err && <p role="alert" className="px-2 py-1 text-xs text-pill-blocked-fg">{err}</p>}
      </div>
    </details>
  );
}

function DeleteAll({ total }: { total?: number }) {
  const wipe = useDeleteAllMemories();
  const [asking, setAsking] = useState(false);
  if (!total && !asking) return null;
  if (!asking)
    return (
      <Button variant="ghost" className="text-pill-blocked-fg" onClick={() => setAsking(true)}>
        Delete everything…
      </Button>
    );
  return (
    <div role="group" aria-label="Confirm delete everything" className="flex flex-wrap items-center gap-2 rounded-md border border-border-strong p-2">
      <span className="flex-1 text-xs text-fg">Forget all {total} memories? This can’t be undone.</span>
      <Button
        variant="primary"
        className="bg-state-blocked"
        disabled={wipe.isPending}
        onClick={() => wipe.mutateAsync().then(() => setAsking(false), () => undefined)}
      >
        Delete everything
      </Button>
      <Button variant="outline" onClick={() => setAsking(false)}>
        Cancel
      </Button>
      {wipe.isError && <p role="alert" className="basis-full text-xs text-pill-blocked-fg">{errorText(wipe.error)}</p>}
    </div>
  );
}

/** "View" from a notice: that entry, open for editing, above the list. */
function Focused({ id, onDone }: { id: string; onDone: () => void }) {
  const m = useMemory(id);
  if (m.isError)
    return (
      <p className="rounded-lg border border-border p-3 text-[13px] text-fg-secondary">
        That memory is no longer saved.
      </p>
    );
  if (!m.data) return <Skeleton className="h-40" />;
  return <Editor key={m.data.id} m={m.data} onDone={onDone} onDeleted={onDone} />;
}

/** The memory manager (mockup frame 2): browse, search, filter, edit, delete and export every entry. */
export function MemoryPanel() {
  const panel = useMemoryPanel();
  const [text, setText] = useState("");
  const [q, setQ] = useState("");
  const [type, setType] = useState<MemoryType | undefined>(undefined);
  const [pages, setPages] = useState(1);
  const [editing, setEditing] = useState<string | undefined>(undefined);
  const count = useMemories();
  // Debounce search typing; a new search or filter starts from the first page.
  useEffect(() => {
    const t = setTimeout(() => {
      setQ(text.trim());
      setPages(1);
    }, 250);
    return () => clearTimeout(t);
  }, [text]);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && !editing && panel.close();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });
  const total = count.data?.total;
  return (
    <aside
      aria-label="Memory"
      className="flex w-[440px] flex-none flex-col border-l border-border-strong bg-surface-sunken max-lg:w-[380px] max-md:absolute max-md:inset-0 max-md:z-20 max-md:w-full"
    >
      <div className="flex items-center gap-2 border-b border-border-subtle px-4 py-2.5">
        <h2 className="type-heading">Memory</h2>
        {total !== undefined && <span className="text-xs text-fg-tertiary">{total} entries</span>}
        <span className="flex-1" />
        <ExportMenu />
        <IconButton icon="x" label="Close memory" onClick={panel.close} />
      </div>
      <div className="flex flex-col gap-2 px-4 pt-3">
        <TextInput type="search" aria-label="Search memories" placeholder="Search memories" value={text} onChange={(e) => setText(e.target.value)} />
        <div role="group" aria-label="Filter by type" className="flex flex-wrap gap-1">
          {[undefined, ...MEMORY_TYPES].map((t) => (
            <button
              key={t ?? "all"}
              type="button"
              aria-pressed={type === t}
              onClick={() => {
                setType(t);
                setPages(1);
              }}
              className={cx(
                "rounded-md px-2.5 py-1 text-xs",
                type === t ? "bg-selected text-fg" : "text-fg-secondary hover:bg-hover",
              )}
            >
              {t ? typeLabel(t) : "All"}
            </button>
          ))}
        </div>
      </div>
      <div className="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto px-4 py-3">
        {panel.focus && <Focused key={panel.focus} id={panel.focus} onDone={() => panel.show()} />}
        {Array.from({ length: pages }, (_, i) => (
          <Page
            key={i}
            q={q}
            type={type}
            offset={i * 50}
            hide={panel.focus}
            editing={editing}
            setEditing={setEditing}
            last={i === pages - 1}
            onMore={() => setPages((n) => n + 1)}
          />
        ))}
      </div>
      <div className="flex flex-col gap-2 border-t border-border-subtle px-4 py-3">
        <DeleteAll total={total} />
        <p className="text-[11.5px] text-fg-tertiary">
          Private to you. Never shared with the team wiki. Secrets and tokens are never saved.
        </p>
      </div>
    </aside>
  );
}
