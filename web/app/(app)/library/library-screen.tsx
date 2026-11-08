"use client";

import Link from "next/link";
import { Dialog, DropdownMenu } from "radix-ui";
import { useRef, useState } from "react";
import { libraryItemUrl } from "@/lib/api/client";
import {
  useClearLibraryReference,
  useDeleteLibraryItem,
  useLibrary,
  useSetLibraryReference,
  useUploadLibraryItem,
  type LibraryItem,
} from "@/lib/api/library-hooks";
import { usePersonas } from "@/lib/api/persona-hooks";
import { cx } from "@/lib/cx";
import { formatBytes } from "@/lib/files";
import { timelineTime } from "@/lib/format";
import { LIBRARY_UPLOAD_ACCEPT, checkLibraryUpload, referenceBadge, referenceCaption, referenceChoices } from "@/lib/library-ui";
import { floatPanel } from "@/components/domain/pickers";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { Banner, EmptyState, Skeleton } from "@/components/ui/primitives";
import { Content, TopBar } from "@/components/shell/topbar";
import { Tabs } from "@/components/ui/controls";
import { Fridge } from "./fridge";

/** Where an item came from: its persona and chat, while they still exist, or the person's own upload. */
function Source({ item }: { item: LibraryItem }) {
  if (item.source === "upload") return <span className="min-w-0 truncate text-xs text-fg-tertiary">Uploaded by you</span>;
  const chat = item.chat_id && item.project_key ? `/chat/${item.chat_id}?project=${encodeURIComponent(item.project_key)}` : undefined;
  return (
    <span className="min-w-0 truncate text-xs text-fg-tertiary">
      {item.persona_name ?? "Assistant"}
      {" · "}
      {chat ? (
        <Link href={chat} className="text-accent hover:underline">
          {item.chat_title || "Open chat"}
        </Link>
      ) : (
        "chat deleted"
      )}
    </span>
  );
}

/**
 * "Use as reference" (for the person or one of their personas) and "Clear
 * reference": new images of that persona or person keep this likeness.
 */
function ReferenceActions({ item }: { item: LibraryItem }) {
  const personas = usePersonas();
  const set = useSetLibraryReference();
  const clear = useClearLibraryReference();
  const choices = referenceChoices(item, personas.data?.personas ?? []);
  const busy = set.isPending || clear.isPending;
  const error = set.error ?? clear.error;
  return (
    <>
      {error && <span className="text-xs text-pill-blocked-fg">{error.message}</span>}
      <DropdownMenu.Root>
        <DropdownMenu.Trigger asChild>
          <Button variant="secondary" icon="users" disabled={busy}>
            Use as reference
          </Button>
        </DropdownMenu.Trigger>
        <DropdownMenu.Portal>
          <DropdownMenu.Content align="end" sideOffset={4} className={cx(floatPanel, "w-60")}>
            {choices.map((c) => (
              <DropdownMenu.Item
                key={c.key}
                onSelect={() => set.mutate({ itemId: item.id, role: c.role, personaId: c.personaId })}
                className="flex h-8 items-center truncate rounded-md px-2 outline-none data-[highlighted]:bg-hover"
              >
                {c.label}
              </DropdownMenu.Item>
            ))}
          </DropdownMenu.Content>
        </DropdownMenu.Portal>
      </DropdownMenu.Root>
      {referenceBadge(item) && (
        <Button variant="ghost" disabled={busy} onClick={() => clear.mutate(item.id)}>
          Clear reference
        </Button>
      )}
    </>
  );
}

/** Marks a current reference image on its tile. */
function ReferenceBadge({ item }: { item: LibraryItem }) {
  const who = referenceBadge(item);
  if (!who) return null;
  return (
    <span
      data-testid="library-reference"
      title={`New images keep this likeness of ${who === "You" ? "you" : who}`}
      className="absolute left-2 top-2 max-w-[calc(100%-16px)] truncate rounded-md bg-surface-raised px-1.5 py-0.5 text-[11px] text-fg shadow-[var(--shadow-float)]"
    >
      {who} · reference
    </span>
  );
}

/** One image tile; a click opens the full image with Download, Remove and the reference actions. */
function ImageTile({ item }: { item: LibraryItem }) {
  const del = useDeleteLibraryItem();
  const url = libraryItemUrl(item.id);
  return (
    <li className="flex min-w-0 flex-col gap-1.5" data-testid="library-item">
      <Dialog.Root>
        <Dialog.Trigger asChild>
          <button
            type="button"
            aria-label={`Open ${item.title}`}
            className="relative aspect-square overflow-hidden rounded-lg border border-border bg-selected outline-none focus-visible:ring-2 focus-visible:ring-accent"
          >
            {/* A same-origin, private URL: next/image would only add a proxy hop. */}
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img src={url} alt={item.title} loading="lazy" className="size-full object-cover" />
            <ReferenceBadge item={item} />
          </button>
        </Dialog.Trigger>
        <Dialog.Portal>
          <Dialog.Overlay className="fixed inset-0 z-40 bg-scrim" />
          <Dialog.Content
            aria-describedby={undefined}
            className="fixed left-1/2 top-1/2 z-50 flex max-h-[92vh] w-[min(1100px,calc(100%-32px))] -translate-x-1/2 -translate-y-1/2 flex-col gap-3 rounded-xl border border-border-float bg-surface-raised p-4 shadow-[var(--shadow-float)] outline-none"
          >
            <div className="flex flex-wrap items-center gap-2">
              <div className="flex min-w-0 flex-1 flex-col">
                <Dialog.Title className="truncate text-[13px] text-fg">{item.title}</Dialog.Title>
                <span className="truncate text-xs text-fg-tertiary">
                  {item.filename} · {formatBytes(item.size_bytes)} · {timelineTime(item.created_at)}
                  {referenceCaption(item) && ` · ${referenceCaption(item)}`}
                </span>
              </div>
              <ReferenceActions item={item} />
              <a
                href={libraryItemUrl(item.id, true)}
                download={item.filename}
                className="inline-flex h-7 items-center gap-1.5 rounded-md border border-btn-secondary-border bg-btn-secondary px-2.5 text-xs text-fg hover:bg-hover"
              >
                <Icon name="download" size={14} />
                Download
              </a>
              <Button
                variant="ghost"
                icon="trash"
                disabled={del.isPending}
                onClick={() => {
                  const msg =
                    item.source === "upload"
                      ? "Remove this image from your Library?"
                      : "Remove this image from your Library? It stays in its chat while that chat exists.";
                  if (window.confirm(msg)) del.mutate(item.id);
                }}
              >
                Remove
              </Button>
              <Dialog.Close asChild>
                <button type="button" aria-label="Close" className="text-fg-tertiary hover:text-fg">
                  <Icon name="x" size={15} />
                </button>
              </Dialog.Close>
            </div>
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img src={url} alt={item.title} className="min-h-0 max-h-[80vh] w-full object-contain" />
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>
      <span className="truncate text-[13px] text-fg">{item.title}</span>
      <Source item={item} />
    </li>
  );
}

/** Adds an image of the person's own, e.g. a photo to use as their reference. */
function UploadButton() {
  const input = useRef<HTMLInputElement>(null);
  const upload = useUploadLibraryItem();
  const [error, setError] = useState<string | null>(null);
  function take(files: FileList | null) {
    const f = files?.[0];
    if (!f) return;
    const bad = checkLibraryUpload(f);
    if (bad) return setError(bad);
    setError(null);
    upload.mutate(f, { onError: (e) => setError(e.message || "Couldn’t upload the image.") });
  }
  return (
    <div className="flex shrink-0 items-center gap-2">
      {error && <span className="text-xs text-pill-blocked-fg">{error}</span>}
      <input
        ref={input}
        type="file"
        accept={LIBRARY_UPLOAD_ACCEPT}
        hidden
        aria-label="Image to upload"
        onChange={(e) => {
          take(e.target.files);
          e.target.value = "";
        }}
      />
      <Button variant="secondary" icon="upload" disabled={upload.isPending} onClick={() => input.current?.click()}>
        {upload.isPending ? "Uploading…" : "Upload"}
      </Button>
    </div>
  );
}

const grid = "grid grid-cols-[repeat(auto-fill,minmax(180px,1fr))] gap-4";

export function LibraryScreen() {
  const lib = useLibrary("image");
  const items = lib.data?.pages.flatMap((p) => p.items) ?? [];
  const [tab, setTab] = useState("images");
  return (
    <>
      <TopBar crumbs={[{ label: "Library" }]} />
      <Content>
        <div className="flex flex-col gap-5">
          <div className="flex items-start gap-4">
            <div className="min-w-0 flex-1 space-y-1">
              <h1 className="type-display">Library</h1>
              <p className="text-sm text-fg-secondary">
                Keepsakes from your Assistant chats, private to you. Images your personas generate are saved here automatically and stay after their chat is deleted.
                Use an image as a persona&apos;s reference, or a photo as yours, and new images of them keep that likeness.
              </p>
            </div>
            <UploadButton />
          </div>
          <Tabs label="Library" value={tab} onChange={setTab} items={[{ key: "images", label: "Images" }, { key: "fridge", label: "Fridge" }]} />
          {tab === "fridge" ? (
            <Fridge />
          ) : lib.isError ? (
            <Banner tone="blocked" title="Couldn't load your Library" actions={<Button onClick={() => lib.refetch()}>Retry</Button>}>
              {lib.error.message}
            </Banner>
          ) : lib.isPending ? (
            <ul aria-busy="true" aria-label="Loading your Library" className={grid}>
              {[0, 1, 2, 3, 4].map((i) => (
                <li key={i} className="flex flex-col gap-1.5">
                  <Skeleton className="aspect-square w-full rounded-lg" />
                  <Skeleton className="h-3 w-2/3" />
                </li>
              ))}
            </ul>
          ) : items.length === 0 ? (
            <EmptyState icon="library" title="Nothing here yet">
              When a persona that can generate images makes one in a chat, it&apos;s added here. You can also upload your own, like a photo of yourself to use as your reference.
            </EmptyState>
          ) : (
            <>
              <ul className={grid}>
                {items.map((it) => (
                  <ImageTile key={it.id} item={it} />
                ))}
              </ul>
              {lib.hasNextPage && (
                <div className="flex justify-center">
                  <Button onClick={() => lib.fetchNextPage()} disabled={lib.isFetchingNextPage}>
                    {lib.isFetchingNextPage ? "Loading…" : "Show more"}
                  </Button>
                </div>
              )}
            </>
          )}
        </div>
      </Content>
    </>
  );
}
