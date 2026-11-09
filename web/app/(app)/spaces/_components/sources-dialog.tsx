"use client";

import Link from "next/link";
import { Dialog, DropdownMenu } from "radix-ui";
import { useRef, useState, type DragEvent, type FormEvent, type ReactNode } from "react";
import { useLibrary } from "@/lib/api/library-hooks";
import { useAddSpaceSource, useDeleteSpaceSource, useMyCanvases, useSpaceSources, useUploadSpaceSource, type SpaceSource } from "@/lib/api/space-hooks";
import { cx } from "@/lib/cx";
import { formatTokens, sourceDetail, sourceName, type SourceKind } from "@/lib/spaces";
import { floatPanel } from "@/components/domain/pickers";
import { Button } from "@/components/ui/button";
import { Icon, type IconName } from "@/components/ui/icon";

const errorText = (e: unknown) => (e instanceof Error ? e.message : "Something went wrong. Please try again.");

export const SOURCE_ICON: Record<SourceKind, IconName> = {
  file: "artifacts",
  text: "edit",
  website: "compass",
  library: "library",
  link: "link",
  canvas: "board",
  research: "search",
};

/** A source's kind icon on a small tile. */
export function SourceIcon({ kind, size = 22 }: { kind: string; size?: number }) {
  return (
    <span aria-hidden className="grid flex-none place-items-center rounded-[5px] bg-pill-neutral-bg text-fg-secondary" style={{ width: size, height: size }}>
      <Icon name={SOURCE_ICON[kind as SourceKind] ?? "artifacts"} size={Math.round(size * 0.6)} />
    </span>
  );
}

type Mode = "drop" | "text" | "website" | "library" | "link" | "canvas";

const OPTIONS: { mode: Mode | "upload"; icon: IconName; label: string }[] = [
  { mode: "upload", icon: "plus", label: "Upload files" },
  { mode: "text", icon: "edit", label: "Paste text" },
  { mode: "website", icon: "compass", label: "Add website" },
  { mode: "library", icon: "library", label: "From Library" },
  { mode: "link", icon: "link", label: "From saved links" },
  { mode: "canvas", icon: "board", label: "From a canvas" },
];

const ACCEPT = ".pdf,.png,.jpg,.jpeg,.gif,.webp,.txt,.md,.markdown,.csv,.json";

/** Inline, Searched or Extracting, as the source rows show it. */
function Delivery({ state }: { state: "inline" | "searched" | "extracting" }) {
  const look = {
    inline: { label: "Inline", className: "bg-pill-done-bg text-pill-done-fg", title: "Offered to every turn to read" },
    searched: { label: "Searched", className: "text-state-progress", title: "Long, or past the inline budget: turns search inside it" },
    extracting: { label: "Extracting", className: "bg-pill-neutral-bg text-fg-secondary", title: "Reading the file" },
  }[state];
  return (
    <span
      title={look.title}
      className={cx("inline-flex h-[20px] flex-none items-center rounded-[10px] px-[10px] text-[11px]", look.className)}
      style={state === "searched" ? { background: "color-mix(in srgb, var(--state-progress) 15%, transparent)" } : undefined}
    >
      {look.label}
    </span>
  );
}

function SourceRow({ s, spaceId }: { s: SpaceSource; spaceId: string }) {
  const del = useDeleteSpaceSource(spaceId);
  return (
    <li className="flex h-[44px] items-center gap-[12px] border-b border-border-row" data-testid="space-source">
      <SourceIcon kind={s.kind} />
      <span className="min-w-[0px] flex-1 truncate text-[13px] text-fg" title={s.url ?? s.title}>
        {sourceName(s)}
      </span>
      <span className="w-[160px] flex-none truncate text-[11.5px] text-fg-tertiary max-sm:hidden">{sourceDetail(s)}</span>
      <span className="flex w-[72px] flex-none justify-end">
        <Delivery state={s.delivery} />
      </span>
      <DropdownMenu.Root>
        <DropdownMenu.Trigger asChild>
          <button type="button" aria-label={`More for ${sourceName(s)}`} className="grid size-[28px] flex-none place-items-center rounded-md text-fg-tertiary hover:bg-hover hover:text-fg">
            <Icon name="dots" size={14} />
          </button>
        </DropdownMenu.Trigger>
        <DropdownMenu.Portal>
          <DropdownMenu.Content align="end" sideOffset={4} className={cx(floatPanel, "w-[176px]")}>
            {s.url && (
              <DropdownMenu.Item asChild className="flex h-[32px] cursor-default items-center gap-[8px] rounded-md px-[8px] outline-none data-[highlighted]:bg-hover">
                <a href={s.url} target="_blank" rel="noreferrer noopener">
                  <Icon name="link" size={13} /> Open page
                </a>
              </DropdownMenu.Item>
            )}
            {s.chat_id && (
              <DropdownMenu.Item asChild className="flex h-[32px] cursor-default items-center gap-[8px] rounded-md px-[8px] outline-none data-[highlighted]:bg-hover">
                <Link href={`/chat/${s.chat_id}`}>
                  <Icon name="msg" size={13} /> Open research chat
                </Link>
              </DropdownMenu.Item>
            )}
            <DropdownMenu.Item
              onSelect={() => del.mutate(s.id)}
              className="flex h-[32px] cursor-default items-center gap-[8px] rounded-md px-[8px] text-pill-blocked-fg outline-none data-[highlighted]:bg-hover"
            >
              <Icon name="trash" size={13} /> Remove from space
            </DropdownMenu.Item>
          </DropdownMenu.Content>
        </DropdownMenu.Portal>
      </DropdownMenu.Root>
    </li>
  );
}

/** A file still uploading, shown as Extracting until its row arrives. */
function PendingRow({ name, error, onDismiss }: { name: string; error?: string; onDismiss: () => void }) {
  return (
    <li className="flex min-h-[44px] flex-wrap items-center gap-[12px] border-b border-border-row py-[6px]" data-testid="space-source-pending">
      <SourceIcon kind="file" />
      <span className="min-w-[0px] flex-1 truncate text-[13px] text-fg-secondary">{name}</span>
      {error ? (
        <>
          <span role="alert" className="text-[11.5px] text-pill-blocked-fg">{error}</span>
          <button type="button" onClick={onDismiss} aria-label={`Dismiss ${name}`} className="grid size-[28px] place-items-center rounded-md text-fg-tertiary hover:bg-hover">
            <Icon name="x" size={12} />
          </button>
        </>
      ) : (
        <>
          <span className="w-[160px] flex-none text-[11.5px] text-fg-tertiary max-sm:hidden">File · reading…</span>
          <span className="flex w-[72px] flex-none justify-end">
            <Delivery state="extracting" />
          </span>
          <span className="size-[28px] flex-none" />
        </>
      )}
    </li>
  );
}

function Panel({ title, onBack, children }: { title: string; onBack: () => void; children: ReactNode }) {
  return (
    <div className="flex min-h-[100px] flex-col gap-[10px] rounded-xl border border-border-strong bg-surface-sunken p-[14px]">
      <div className="flex items-center justify-between">
        <span className="text-[13px] font-medium text-fg">{title}</span>
        <button type="button" onClick={onBack} className="text-[12px] text-fg-secondary hover:text-fg">
          Cancel
        </button>
      </div>
      {children}
    </div>
  );
}

const field = "w-full rounded-md border border-border-strong bg-surface px-[10px] text-[13px] text-fg placeholder:text-fg-tertiary focus:border-accent focus:outline-none";

function TextForm({ spaceId, onDone }: { spaceId: string; onDone: () => void }) {
  const add = useAddSpaceSource(spaceId);
  const [title, setTitle] = useState("");
  const [text, setText] = useState("");
  function submit(e: FormEvent) {
    e.preventDefault();
    if (!text.trim()) return;
    add.mutate({ kind: "text", text, ...(title.trim() ? { title: title.trim() } : {}) }, { onSuccess: onDone });
  }
  return (
    <Panel title="Paste text" onBack={onDone}>
      <form onSubmit={submit} className="flex flex-col gap-[8px]">
        <input aria-label="Title" className={cx(field, "h-[30px]")} placeholder="Title (optional)" value={title} onChange={(e) => setTitle(e.target.value)} />
        <textarea aria-label="Text" autoFocus className={cx(field, "h-[128px] resize-y py-[8px]")} placeholder="Paste notes, a plan, an article…" value={text} onChange={(e) => setText(e.target.value)} />
        {add.isError && <p role="alert" className="text-[12px] text-pill-blocked-fg">{errorText(add.error)}</p>}
        <div className="flex justify-end">
          <Button type="submit" variant="primary" disabled={add.isPending || !text.trim()}>{add.isPending ? "Adding…" : "Add text"}</Button>
        </div>
      </form>
    </Panel>
  );
}

function WebsiteForm({ spaceId, onDone }: { spaceId: string; onDone: () => void }) {
  const add = useAddSpaceSource(spaceId);
  const [url, setUrl] = useState("");
  function submit(e: FormEvent) {
    e.preventDefault();
    const u = url.trim();
    if (!u) return;
    add.mutate({ kind: "website", url: /^https?:\/\//i.test(u) ? u : `https://${u}` }, { onSuccess: onDone });
  }
  return (
    <Panel title="Add website" onBack={onDone}>
      <form onSubmit={submit} className="flex flex-col gap-[8px]">
        <div className="flex gap-[8px]">
          <input aria-label="Web address" autoFocus className={cx(field, "h-[30px] flex-1")} placeholder="https://examine.com/creatine" value={url} onChange={(e) => setUrl(e.target.value)} />
          <Button type="submit" variant="primary" disabled={add.isPending || !url.trim()}>{add.isPending ? "Reading…" : "Add"}</Button>
        </div>
        <p className="text-[11.5px] text-fg-tertiary">The page is read once now and kept as text.</p>
        {add.isError && <p role="alert" className="text-[12px] text-pill-blocked-fg">{errorText(add.error)}</p>}
      </form>
    </Panel>
  );
}

function PickList<T extends { id: string }>({ items, loading, empty, render, onPick, busy }: { items: T[]; loading: boolean; empty: string; render: (t: T) => ReactNode; onPick: (t: T) => void; busy: boolean }) {
  if (loading) return <p className="px-[4px] py-[8px] text-[12px] text-fg-tertiary">Loading…</p>;
  if (items.length === 0) return <p className="px-[4px] py-[8px] text-[12px] text-fg-secondary">{empty}</p>;
  return (
    <ul className="flex max-h-[180px] flex-col gap-px overflow-y-auto">
      {items.map((t) => (
        <li key={t.id}>
          <button type="button" disabled={busy} onClick={() => onPick(t)} className="flex h-[32px] w-full items-center gap-[10px] rounded-md px-[8px] text-left text-[13px] hover:bg-hover disabled:opacity-50">
            {render(t)}
            <span className="ml-auto flex-none text-[12px] text-accent">Add</span>
          </button>
        </li>
      ))}
    </ul>
  );
}

function LibraryPicker({ spaceId, links, onDone }: { spaceId: string; links: boolean; onDone: () => void }) {
  const add = useAddSpaceSource(spaceId);
  const lib = useLibrary(links ? "link" : undefined);
  const items = (lib.data?.pages ?? []).flatMap((p) => p.items).filter((i) => (links ? i.kind === "link" : i.kind !== "link"));
  return (
    <Panel title={links ? "From saved links" : "From Library"} onBack={onDone}>
      <PickList
        items={items}
        loading={lib.isPending}
        busy={add.isPending}
        empty={links ? "No saved links yet." : "Nothing in your Library yet."}
        onPick={(i) => add.mutate({ kind: links ? "link" : "library", item_id: i.id }, { onSuccess: onDone })}
        render={(i) => (
          <>
            <SourceIcon kind={i.kind === "link" ? "link" : "library"} size={18} />
            <span className="min-w-[0px] truncate">{i.title}</span>
            <span className="truncate text-[11.5px] text-fg-tertiary">{i.kind === "link" ? (i.site ?? i.url) : i.kind}</span>
          </>
        )}
      />
      {add.isPending && <p className="text-[12px] text-fg-tertiary">{links ? "Reading the page…" : "Adding…"}</p>}
      {add.isError && <p role="alert" className="text-[12px] text-pill-blocked-fg">{errorText(add.error)}</p>}
    </Panel>
  );
}

function CanvasPicker({ spaceId, onDone }: { spaceId: string; onDone: () => void }) {
  const add = useAddSpaceSource(spaceId);
  const canvases = useMyCanvases();
  return (
    <Panel title="From a canvas" onBack={onDone}>
      <PickList
        items={canvases.data?.canvases ?? []}
        loading={canvases.isPending}
        busy={add.isPending}
        empty="No canvases yet. Ask a persona for a document or mockup to make one."
        onPick={(c) => add.mutate({ kind: "canvas", canvas_id: c.id }, { onSuccess: onDone })}
        render={(c) => (
          <>
            <SourceIcon kind="canvas" size={18} />
            <span className="min-w-[0px] truncate">{c.title}</span>
            <span className="truncate text-[11.5px] text-fg-tertiary">{c.chat_title || "Untitled chat"}</span>
          </>
        )}
      />
      {add.isError && <p role="alert" className="text-[12px] text-pill-blocked-fg">{errorText(add.error)}</p>}
    </Panel>
  );
}

type Pending = { key: string; name: string; error?: string };

/** Mockup 4: add sources by upload, paste, page, Library, saved link or canvas; see each one's delivery and the inline budget. */
export function SourcesDialog({ spaceId, spaceName, open, onOpenChange }: { spaceId: string; spaceName: string; open: boolean; onOpenChange: (open: boolean) => void }) {
  const sources = useSpaceSources(open ? spaceId : undefined);
  const upload = useUploadSpaceSource(spaceId);
  const [mode, setMode] = useState<Mode>("drop");
  const [pending, setPending] = useState<Pending[]>([]);
  const [over, setOver] = useState(false);
  const picker = useRef<HTMLInputElement>(null);
  const list = sources.data?.sources ?? [];
  const budget = sources.data?.inline_budget ?? 40000;
  const used = sources.data?.inline_tokens ?? 0;

  function send(files: File[]) {
    setMode("drop");
    for (const f of files) {
      const key = `${f.name}-${f.size}-${Math.random()}`;
      setPending((p) => [...p, { key, name: f.name }]);
      upload.mutateAsync(f).then(
        () => setPending((p) => p.filter((x) => x.key !== key)),
        (e) => setPending((p) => p.map((x) => (x.key === key ? { ...x, error: errorText(e) } : x))),
      );
    }
  }
  const drop = {
    onDragOver: (e: DragEvent) => {
      if (!e.dataTransfer.types.includes("Files")) return;
      e.preventDefault();
      setOver(true);
    },
    onDragLeave: () => setOver(false),
    onDrop: (e: DragEvent) => {
      e.preventDefault();
      setOver(false);
      send(Array.from(e.dataTransfer.files));
    },
  };
  const back = () => setMode("drop");
  return (
    <Dialog.Root
      open={open}
      onOpenChange={(o) => {
        onOpenChange(o);
        if (!o) {
          setMode("drop");
          setPending((p) => p.filter((x) => !x.error));
        }
      }}
    >
      <Dialog.Portal>
        <Dialog.Overlay className="motion-scrim fixed inset-0 z-40 bg-scrim" />
        <Dialog.Content className="motion-dialog fixed left-1/2 top-[8vh] z-50 flex max-h-[84vh] w-[min(860px,calc(100%-32px))] -translate-x-1/2 flex-col overflow-y-auto rounded-2xl border border-border-float bg-surface-raised px-[28px] pb-[20px] pt-[24px] shadow-[var(--shadow-float)] outline-none">
          <div className="flex items-start justify-between">
            <div className="flex flex-col gap-[4px]">
              <Dialog.Title className="text-[18px] font-semibold text-fg">Sources</Dialog.Title>
              <Dialog.Description className="text-[12.5px] text-fg-tertiary">{spaceName} · available to every chat in this space</Dialog.Description>
            </div>
            <Dialog.Close className="grid size-[28px] place-items-center rounded-md text-fg-secondary hover:bg-hover hover:text-fg" aria-label="Close">
              <Icon name="x" size={14} strokeWidth={1.6} />
            </Dialog.Close>
          </div>

          <div className="mt-[24px] flex gap-[24px] max-md:flex-col">
            <div className="flex w-[240px] flex-none flex-col gap-[10px] max-md:w-full">
              {OPTIONS.map((o) => (
                <button
                  key={o.mode}
                  type="button"
                  aria-pressed={mode === o.mode}
                  onClick={() => (o.mode === "upload" ? picker.current?.click() : setMode(o.mode))}
                  className={cx(
                    "flex h-[48px] items-center gap-[12px] rounded-[10px] border px-[20px] text-left text-[13.5px] text-fg hover:bg-hover",
                    mode === o.mode ? "border-accent bg-accent-soft" : "border-border-strong bg-surface",
                  )}
                >
                  <Icon name={o.icon} size={15} className="text-fg-secondary" />
                  {o.label}
                </button>
              ))}
              <input
                ref={picker}
                type="file"
                multiple
                accept={ACCEPT}
                hidden
                aria-label="Upload files"
                onChange={(e) => {
                  send(Array.from(e.target.files ?? []));
                  e.target.value = "";
                }}
              />
            </div>

            <div className="flex min-w-[0px] flex-1 flex-col">
              {mode === "drop" ? (
                <button
                  type="button"
                  onClick={() => picker.current?.click()}
                  {...drop}
                  className={cx(
                    "flex h-[100px] flex-col items-center justify-center gap-[6px] rounded-xl border border-dashed text-center",
                    over ? "border-accent bg-accent-soft" : "border-border-hover bg-surface-sunken hover:bg-hover",
                  )}
                >
                  <span className="text-[14px] text-fg-secondary">Drop files here</span>
                  <span className="text-[12px] text-fg-tertiary">PDF, Markdown, text and images · up to 20 MB each</span>
                </button>
              ) : mode === "text" ? (
                <TextForm spaceId={spaceId} onDone={back} />
              ) : mode === "website" ? (
                <WebsiteForm spaceId={spaceId} onDone={back} />
              ) : mode === "canvas" ? (
                <CanvasPicker spaceId={spaceId} onDone={back} />
              ) : (
                <LibraryPicker key={mode} spaceId={spaceId} links={mode === "link"} onDone={back} />
              )}

              <div className="mt-[28px] border-b border-border-subtle pb-[10px] text-[12px] text-fg-secondary">In this space · {list.length + pending.filter((p) => !p.error).length}</div>
              {sources.isError ? (
                <p role="alert" className="py-[12px] text-[12px] text-pill-blocked-fg">{errorText(sources.error)}</p>
              ) : (
                <ul aria-label="Sources in this space">
                  {list.map((s) => (
                    <SourceRow key={s.id} s={s} spaceId={spaceId} />
                  ))}
                  {pending.map((p) => (
                    <PendingRow key={p.key} name={p.name} error={p.error} onDismiss={() => setPending((ps) => ps.filter((x) => x.key !== p.key))} />
                  ))}
                  {sources.data && list.length === 0 && pending.length === 0 && (
                    <li className="py-[12px] text-[12.5px] text-fg-secondary">Nothing here yet. Every chat in {spaceName} will be able to use what you add.</li>
                  )}
                </ul>
              )}

              <p className="mt-[20px] text-[12px] text-fg-secondary">
                Inline context: {formatTokens(used, true)} of {formatTokens(budget)} tokens
              </p>
              <div className="mt-[10px] h-[6px] overflow-hidden rounded-[3px] bg-pill-neutral-bg" role="meter" aria-label="Inline context" aria-valuemin={0} aria-valuemax={budget} aria-valuenow={used}>
                <div className="h-full rounded-[3px] bg-accent" style={{ width: `${Math.min(100, (used / budget) * 100)}%` }} />
              </div>
              <p className="mt-[12px] text-[11.5px] text-fg-tertiary">Large sources, or anything past the budget, are searched when needed instead of sent every turn.</p>
              <p className="mt-[6px] text-[11.5px] text-fg-tertiary">Deep research reports written in this space’s chats are added here when they’re saved. Start one with Research in the space’s composer.</p>
            </div>
          </div>
          <div className="mt-[20px] flex justify-end">
            <Dialog.Close asChild>
              <Button variant="primary" className="!h-[34px] !px-[24px]">Done</Button>
            </Dialog.Close>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
