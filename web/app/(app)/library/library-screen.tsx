"use client";

import Link from "next/link";
import { Dialog } from "radix-ui";
import { libraryItemUrl } from "@/lib/api/client";
import { useDeleteLibraryItem, useLibrary, type LibraryItem } from "@/lib/api/library-hooks";
import { formatBytes } from "@/lib/files";
import { timelineTime } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { Banner, EmptyState, Skeleton } from "@/components/ui/primitives";
import { Content, TopBar } from "@/components/shell/topbar";
import { Tabs } from "@/components/ui/controls";
import { useState } from "react";
import { Fridge } from "./fridge";

/** Where an item came from: its persona and chat, while they still exist. */
function Source({ item }: { item: LibraryItem }) {
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

/** One image tile; a click opens the full image with Download and Remove. */
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
            className="aspect-square overflow-hidden rounded-lg border border-border bg-selected outline-none focus-visible:ring-2 focus-visible:ring-accent"
          >
            {/* A same-origin, private URL: next/image would only add a proxy hop. */}
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img src={url} alt={item.title} loading="lazy" className="size-full object-cover" />
          </button>
        </Dialog.Trigger>
        <Dialog.Portal>
          <Dialog.Overlay className="fixed inset-0 z-40 bg-scrim" />
          <Dialog.Content
            aria-describedby={undefined}
            className="fixed left-1/2 top-1/2 z-50 flex max-h-[92vh] w-[min(1100px,calc(100%-32px))] -translate-x-1/2 -translate-y-1/2 flex-col gap-3 rounded-xl border border-border-float bg-surface-raised p-4 shadow-[var(--shadow-float)] outline-none"
          >
            <div className="flex items-center gap-2">
              <div className="flex min-w-0 flex-1 flex-col">
                <Dialog.Title className="truncate text-[13px] text-fg">{item.title}</Dialog.Title>
                <span className="truncate text-xs text-fg-tertiary">
                  {item.filename} · {formatBytes(item.size_bytes)} · {timelineTime(item.created_at)}
                </span>
              </div>
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
                  if (window.confirm("Remove this image from your Library? It stays in its chat while that chat exists.")) del.mutate(item.id);
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
          <div className="space-y-1">
            <h1 className="type-display">Library</h1>
            <p className="text-sm text-fg-secondary">
              Keepsakes from your Assistant chats, private to you. Images your personas generate are saved here automatically and stay after their chat is deleted.
            </p>
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
              When a persona that can generate images makes one in a chat, it&apos;s added here.
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
