"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { Dialog, DropdownMenu } from "radix-ui";
import { useMemo, useState, type FormEvent, type KeyboardEvent } from "react";
import { useCreateDeck, useDecks, useDeleteDeck, useDuplicateDeck, useRenameDeck, type DeckSummary } from "@/lib/api/slides-hooks";
import { ago } from "@/lib/format";
import { cx } from "@/lib/cx";
import { assetHref, SlideSvg, useSlideFonts } from "@/lib/slides/render";
import { THEMES, parseDeck, type Deck, type ThemeKey } from "@/lib/slides/schema";
import { floatPanel } from "@/components/domain/pickers";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { Banner, EmptyState, Skeleton } from "@/components/ui/primitives";
import { TopBar } from "@/components/shell/topbar";

const errorText = (e: unknown) => (e instanceof Error ? e.message : "Something went wrong. Please try again.");
const menuItem = "flex cursor-pointer select-none items-center rounded-md px-2.5 py-1.5 text-[13px] text-fg outline-none data-[highlighted]:bg-hover";

/**
 * Text that reads like a request rather than a title starts the deck and asks
 * the persona to draft it.
 */
const isBrief = (s: string) => s.trim().split(/\s+/).length > 8 || /^(make|create|draft|build|write|turn|a deck|slides)\b/i.test(s.trim());

/** The Slides app: the person's decks and a way to start one (mockup 1/6). */
export function DecksScreen() {
  const decks = useDecks();
  const create = useCreateDeck();
  const router = useRouter();
  const [title, setTitle] = useState("");
  const [theme, setTheme] = useState<ThemeKey>("light");
  const fonts = useSlideFonts();

  const start = (e?: FormEvent) => {
    e?.preventDefault();
    if (create.isPending) return;
    const brief = isBrief(title) ? title.trim() : "";
    create.mutate(
      { title: brief ? "" : title, theme: theme as "light" },
      { onSuccess: (d) => router.push(`/apps/slides/${d.id}${brief ? `?brief=${encodeURIComponent(brief)}` : ""}`) },
    );
  };

  // Arrow keys move between cards (mockup note 3).
  const onGridKey = (e: KeyboardEvent<HTMLUListElement>) => {
    const cards = [...e.currentTarget.querySelectorAll<HTMLAnchorElement>("a[data-deck-card]")];
    const i = cards.indexOf(document.activeElement as HTMLAnchorElement);
    if (i < 0) return;
    const cols = Math.max(1, Math.round(e.currentTarget.clientWidth / (cards[0]?.offsetWidth || 1)));
    const next = { ArrowRight: i + 1, ArrowLeft: i - 1, ArrowDown: i + cols, ArrowUp: i - cols }[e.key];
    if (next !== undefined && cards[next]) {
      e.preventDefault();
      cards[next].focus();
    }
  };

  return (
    <div className="flex h-dvh flex-col">
      <TopBar crumbs={[{ label: "Apps" }, { label: "Slides" }]} />
      <main className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto flex max-w-[1000px] flex-col gap-5 px-6 py-6 max-sm:px-4">
          <div>
            <h1 className="type-display text-fg">Slides</h1>
            <p className="mt-1 text-[13px] text-fg-secondary">Decks you build and present with a persona. Each deck keeps its own chat.</p>
          </div>
          <form onSubmit={start} className="flex flex-wrap gap-2 rounded-xl border border-border-strong bg-surface p-1 pl-3 focus-within:border-accent">
            <input
              aria-label="New deck title or brief"
              className="min-w-[200px] flex-1 bg-transparent text-[13.5px] text-fg outline-none placeholder:text-fg-tertiary"
              placeholder="Start a deck: give it a title, or describe it and your persona drafts the slides…"
              maxLength={2000}
              value={title}
              onChange={(e) => setTitle(e.target.value)}
            />
            <label className="flex items-center gap-2 rounded-md border border-border-strong bg-surface-sunken px-2 text-[12px] text-fg-2 max-sm:order-last max-sm:w-full">
              <span aria-hidden className="h-3.5 w-5 rounded-[3px] border border-border-strong" style={{ background: THEMES[theme].colors.background }} />
              <span className="sr-only">Theme</span>
              <select aria-label="Theme" className="h-[26px] bg-transparent outline-none" value={theme} onChange={(e) => setTheme(e.target.value as ThemeKey)}>
                {Object.entries(THEMES).map(([k, t]) => (
                  <option key={k} value={k}>
                    Theme: {t.name}
                  </option>
                ))}
              </select>
            </label>
            <Button type="submit" variant="primary" icon="plus" disabled={create.isPending}>
              {create.isPending ? "Creating…" : "New deck"}
            </Button>
          </form>
          {create.isError && <p role="alert" className="text-[12px] text-pill-blocked-fg">{errorText(create.error)}</p>}

          <div className="flex items-baseline justify-between">
            <h2 className="text-[12.5px] font-medium text-fg-secondary">Recent</h2>
            <span className="text-[12px] text-fg-tertiary">Sorted by last edited</span>
          </div>
          {decks.isError ? (
            <Banner tone="blocked" title="Couldn’t load your decks" actions={<Button onClick={() => void decks.refetch()}>Retry</Button>}>
              {errorText(decks.error)} Your decks are safe.
            </Banner>
          ) : !decks.data ? (
            <div aria-busy="true" className="grid grid-cols-[repeat(auto-fill,minmax(280px,1fr))] gap-4">
              {[0, 1, 2].map((i) => (
                <div key={i} className="rounded-xl border border-border bg-surface p-3">
                  <Skeleton className="aspect-video w-full" />
                  <Skeleton className="mt-3 h-3 w-2/3" />
                  <Skeleton className="mt-2 h-3 w-1/3" />
                </div>
              ))}
            </div>
          ) : decks.data.length === 0 ? (
            <EmptyState icon="slides" title="No decks yet">
              Start one above, or open Slides from Apps any time: every deck you make shows up here.
            </EmptyState>
          ) : (
            <ul aria-label="Decks" className="grid grid-cols-[repeat(auto-fill,minmax(280px,1fr))] gap-4" onKeyDown={onGridKey}>
              {decks.data.map((d) => (
                <DeckCard key={d.id} d={d} fonts={fonts} />
              ))}
            </ul>
          )}
        </div>
      </main>
    </div>
  );
}

function DeckCard({ d, fonts }: { d: DeckSummary; fonts: boolean }) {
  const router = useRouter();
  const rename = useRenameDeck();
  const duplicate = useDuplicateDeck();
  const remove = useDeleteDeck();
  const [renaming, setRenaming] = useState(false);
  const [name, setName] = useState(d.title);
  const [confirming, setConfirming] = useState(false);
  const cover = useMemo<Deck | null>(() => {
    try {
      return d.cover ? parseDeck(d.cover) : null;
    } catch {
      return null;
    }
  }, [d.cover]);
  const error = rename.error ?? duplicate.error ?? remove.error;

  const commitRename = () => {
    setRenaming(false);
    if (name.trim() && name.trim() !== d.title) rename.mutate({ id: d.id, title: name.trim() });
    else setName(d.title);
  };

  return (
    <li className="group relative">
      <Link
        href={`/apps/slides/${d.id}`}
        data-deck-card
        className="flex flex-col rounded-xl border border-border bg-surface p-3 outline-none transition-colors hover:border-border-hover focus-visible:border-accent focus-visible:ring-2 focus-visible:ring-accent/35"
      >
        <span className="block overflow-hidden rounded-[4px] border border-border">
          {cover ? (
            <SlideSvg deck={cover} slide={cover.slides[0]} assetUrl={(a) => assetHref(d.id, a)} fontsReady={fonts} className="block aspect-video w-full" label={`First slide of ${d.title}`} />
          ) : (
            <span className="block aspect-video w-full bg-[#fff]" />
          )}
        </span>
        <span className="mt-3 block truncate pr-8 text-[13.5px] font-medium text-fg">{renaming ? " " : d.title}</span>
        <span className="block text-[12px] text-fg-tertiary">
          Edited {ago(d.updated_at)} · {d.slide_count} {d.slide_count === 1 ? "slide" : "slides"}
        </span>
      </Link>
      {renaming && (
        <form
          className="absolute inset-x-3 top-[calc(100%-52px)]"
          onSubmit={(e) => {
            e.preventDefault();
            commitRename();
          }}
        >
          <input
            autoFocus
            aria-label="Deck title"
            maxLength={200}
            className="h-[26px] w-full rounded-md border border-accent bg-surface px-2 text-[13.5px] text-fg outline-none"
            value={name}
            onChange={(e) => setName(e.target.value)}
            onBlur={commitRename}
            onKeyDown={(e) => {
              if (e.key === "Escape") {
                setName(d.title);
                setRenaming(false);
              }
            }}
          />
        </form>
      )}
      <DropdownMenu.Root>
        <DropdownMenu.Trigger asChild>
          <button
            type="button"
            aria-label={`More actions for ${d.title}`}
            className="absolute bottom-[30px] right-3 grid size-[26px] place-items-center rounded-md text-fg-secondary outline-none hover:bg-hover focus-visible:ring-2 focus-visible:ring-accent data-[state=open]:bg-hover"
          >
            <Icon name="dots" size={15} />
          </button>
        </DropdownMenu.Trigger>
        <DropdownMenu.Portal>
          <DropdownMenu.Content align="end" sideOffset={6} className={cx(floatPanel, "z-50 w-44")}>
            <DropdownMenu.Item className={menuItem} onSelect={() => router.push(`/apps/slides/${d.id}`)}>Open</DropdownMenu.Item>
            <DropdownMenu.Item className={menuItem} onSelect={() => setRenaming(true)}>Rename</DropdownMenu.Item>
            <DropdownMenu.Item className={menuItem} disabled={duplicate.isPending} onSelect={() => duplicate.mutate(d.id)}>Duplicate</DropdownMenu.Item>
            <DropdownMenu.Item className={menuItem} onSelect={() => router.push(`/present/slides/${d.id}`)}>Present</DropdownMenu.Item>
            <DropdownMenu.Separator className="my-1 h-px bg-border-subtle" />
            <DropdownMenu.Item className={cx(menuItem, "text-pill-blocked-fg")} onSelect={() => setConfirming(true)}>Delete…</DropdownMenu.Item>
          </DropdownMenu.Content>
        </DropdownMenu.Portal>
      </DropdownMenu.Root>
      {error && <p role="alert" className="mt-1 text-[12px] text-pill-blocked-fg">{errorText(error)}</p>}
      <Dialog.Root open={confirming} onOpenChange={setConfirming}>
        <Dialog.Portal>
          <Dialog.Overlay className="motion-scrim fixed inset-0 z-40 bg-scrim" />
          <Dialog.Content className="motion-dialog fixed left-1/2 top-[14vh] z-50 w-[min(420px,calc(100%-32px))] -translate-x-1/2 rounded-xl border border-border-float bg-surface-raised p-4 shadow-[var(--shadow-float)] outline-none">
            <Dialog.Title className="type-heading">Delete “{d.title}”?</Dialog.Title>
            <Dialog.Description className="mt-2 text-[13px] text-fg-secondary">The deck, its versions, its images and its chat are deleted. This can’t be undone.</Dialog.Description>
            <div className="mt-4 flex justify-end gap-2">
              <Dialog.Close asChild>
                <Button variant="ghost">Cancel</Button>
              </Dialog.Close>
              <Button
                variant="primary"
                className="bg-state-blocked text-[#fff]"
                disabled={remove.isPending}
                onClick={() => remove.mutate(d.id, { onSuccess: () => setConfirming(false) })}
              >
                {remove.isPending ? "Deleting…" : "Delete deck"}
              </Button>
            </div>
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>
    </li>
  );
}
