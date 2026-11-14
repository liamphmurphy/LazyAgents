"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { Dialog, DropdownMenu } from "radix-ui";
import { useMemo, useRef, useState, type FormEvent, type KeyboardEvent } from "react";
import { useCreateDeck, useDecks, useDeleteDeck, useDuplicateDeck, useRenameDeck, useSlideStarters, type DeckSummary, type SlideStarter, type StarterKey } from "@/lib/api/slides-hooks";
import { ago } from "@/lib/format";
import { cx } from "@/lib/cx";
import { assetHref, SlideSvg, useSlideFonts } from "@/lib/slides/render";
import { MAX_OUTLINE, outlineTitle } from "@/lib/slides/outline";
import { H, THEMES, W, parseDeck, placeholders, type Deck, type ThemeKey } from "@/lib/slides/schema";
import { floatPanel } from "@/components/domain/pickers";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { Banner, EmptyState, Skeleton } from "@/components/ui/primitives";
import { TopBar } from "@/components/shell/topbar";
import { PageHeader } from "@/components/ui/page-header";

const errorText = (e: unknown) => (e instanceof Error ? e.message : "Something went wrong. Please try again.");
const menuItem = "flex cursor-pointer select-none items-center rounded-md px-2.5 py-1.5 text-13 text-fg outline-none data-[highlighted]:bg-hover";

/**
 * Text that reads like a request rather than a title starts the deck and asks
 * the persona to draft it.
 */
const isBrief = (s: string) => s.trim().split(/\s+/).length > 8 || /^(make|create|draft|build|write|turn|a deck|slides)\b/i.test(s.trim());

/** What a new deck starts from: one blank title slide, a starter deck, or an outline the persona drafts. */
type Start = "blank" | "outline" | StarterKey;

/** The Slides app: the person's decks and a way to start one (mockup 1/6; chooser: New deck chooser mockup). */
export function DecksScreen() {
  const decks = useDecks();
  const create = useCreateDeck();
  const router = useRouter();
  const [title, setTitle] = useState("");
  const [theme, setTheme] = useState<ThemeKey>("light");
  const [from, setFrom] = useState<Start>("blank");
  const [outline, setOutline] = useState("");
  const [outlineError, setOutlineError] = useState("");
  const fonts = useSlideFonts();

  const start = (e?: FormEvent) => {
    e?.preventDefault();
    if (create.isPending) return;
    if (from === "outline") {
      // With no title, the outline's first line titles the deck and the persona drafts the rest.
      const lines = outline.split("\n");
      const first = lines.findIndex((l) => l.trim());
      const deckTitle = title.trim() || outlineTitle(outline);
      const rest = (title.trim() ? outline : lines.slice(first + 1).join("\n")).trim();
      if (!rest) return setOutlineError("Add at least one line for a slide after the title.");
      setOutlineError("");
      create.mutate({ title: deckTitle, theme: theme as "light" }, { onSuccess: (d) => router.push(`/apps/slides/${d.id}?outline=${encodeURIComponent(rest)}`) });
      return;
    }
    if (from !== "blank") {
      create.mutate({ title, theme: theme as "light", starter: from }, { onSuccess: (d) => router.push(`/apps/slides/${d.id}`) });
      return;
    }
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
    <div className="flex h-app flex-col">
      <TopBar crumbs={[{ label: "Apps" }, { label: "Slides" }]} />
      <main className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto flex max-w-[1000px] flex-col gap-5 px-6 py-6 max-sm:px-4">
          <PageHeader title="Slides" description="Decks you build and present with a persona. Each deck keeps its own chat." />
          <form onSubmit={start} className="flex flex-wrap gap-2 rounded-xl border border-border-strong bg-surface p-1 pl-3 focus-within:border-accent">
            <input
              aria-label="New deck title or brief"
              className="min-w-[200px] flex-1 bg-transparent text-14 text-fg outline-none placeholder:text-fg-tertiary"
              placeholder="Start a deck: give it a title, or describe it and your persona drafts the slides…"
              maxLength={2000}
              value={title}
              onChange={(e) => setTitle(e.target.value)}
            />
            <label className="flex items-center gap-2 rounded-md border border-border-strong bg-surface-sunken px-2 text-12 text-fg-2 max-sm:order-last max-sm:w-full">
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
          {create.isError && <p role="alert" className="text-12 text-pill-blocked-fg">{errorText(create.error)}</p>}
          <StartChooser value={from} onChange={setFrom} theme={theme} fonts={fonts} />
          {from === "outline" && (
            <form onSubmit={start} className="flex flex-col gap-2 rounded-xl border border-accent bg-surface p-3">
              <div className="flex flex-wrap items-baseline justify-between gap-2">
                <label htmlFor="deck-outline" className="text-12 font-medium text-fg-2">Outline</label>
                <span className="text-12 text-fg-tertiary">One slide per top-level line · indented lines become its points</span>
              </div>
              <textarea
                id="deck-outline"
                autoFocus
                rows={6}
                maxLength={MAX_OUTLINE}
                placeholder={"Q3 roadmap\nWhy now\n  - churn up 4% in EU\nPlan: three bets"}
                className="resize-y rounded-md border border-border bg-surface-sunken px-3 py-2 font-mono text-13 text-fg outline-none placeholder:text-fg-tertiary focus:border-accent"
                value={outline}
                onChange={(e) => setOutline(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) start();
                }}
              />
              {outlineError && <p role="alert" className="text-12 text-pill-blocked-fg">{outlineError}</p>}
              <div className="flex flex-wrap items-center justify-between gap-2">
                <p className="text-12 text-fg-secondary">The deck opens with its title slide; your persona drafts the rest as suggestions. Nothing is added until you accept.</p>
                <Button type="submit" variant="primary" disabled={create.isPending || !outline.trim()}>
                  {create.isPending ? "Creating…" : "✦ Create & draft"}
                </Button>
              </div>
            </form>
          )}

          <div className="flex items-baseline justify-between">
            <h2 className="text-13 font-medium text-fg-secondary">Recent</h2>
            <span className="text-12 text-fg-tertiary">Sorted by last edited</span>
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

const BLANK_COVER: Deck = { schema: 1, size: { w: W, h: H }, theme: THEMES.light, assets: {}, slides: [{ id: "s_blank", layout: "title", elements: placeholders("title") }] };

/**
 * Start from: Blank, the starter decks and Draft from outline, as a radio
 * group (arrow keys move and pick). Previews draw in the chosen theme.
 */
function StartChooser({ value, onChange, theme, fonts }: { value: Start; onChange: (s: Start) => void; theme: ThemeKey; fonts: boolean }) {
  const starters = useSlideStarters();
  const group = useRef<HTMLDivElement>(null);
  const covers = useMemo(
    () =>
      (starters.data ?? []).flatMap((s: SlideStarter) => {
        try {
          return [{ s, deck: parseDeck(s.cover) }];
        } catch {
          return [];
        }
      }),
    [starters.data],
  );
  const options: { key: Start; label: string; detail: string; deck?: Deck }[] = [
    { key: "blank", label: "Blank", detail: "1 slide", deck: BLANK_COVER },
    ...covers.map(({ s, deck }) => ({ key: s.key as Start, label: s.name, detail: `${s.slide_count} slides`, deck })),
    { key: "outline", label: "✦ Draft from outline", detail: "Persona suggests slides" },
  ];
  const onKey = (e: KeyboardEvent<HTMLDivElement>) => {
    const i = options.findIndex((o) => o.key === value);
    const next = { ArrowRight: i + 1, ArrowDown: i + 1, ArrowLeft: i - 1, ArrowUp: i - 1 }[e.key];
    if (next === undefined) return;
    e.preventDefault();
    const o = options[(next + options.length) % options.length];
    onChange(o.key);
    group.current?.querySelector<HTMLButtonElement>(`[data-start="${o.key}"]`)?.focus();
  };

  return (
    <section aria-labelledby="start-from" className="flex flex-col gap-2">
      <div className="flex items-baseline justify-between">
        <h2 id="start-from" className="text-13 font-medium text-fg-secondary">Start from</h2>
        <span className="text-12 text-fg-tertiary max-sm:hidden">Previews use the theme you picked</span>
      </div>
      <div ref={group} role="radiogroup" aria-labelledby="start-from" onKeyDown={onKey} className="grid grid-cols-[repeat(auto-fill,minmax(128px,1fr))] gap-2.5">
        {options.map((o) => {
          const on = o.key === value;
          return (
            <button
              key={o.key}
              type="button"
              role="radio"
              aria-checked={on}
              data-start={o.key}
              tabIndex={on ? 0 : -1}
              onClick={() => onChange(o.key)}
              className={cx(
                "relative flex flex-col rounded-xl border bg-surface p-2 text-left outline-none transition-colors focus-visible:ring-2 focus-visible:ring-accent/35",
                on ? "border-accent ring-1 ring-accent" : "border-border hover:border-border-hover",
                o.key === "outline" && !on && "border-dashed",
              )}
            >
              <span className="block overflow-hidden rounded-[4px] border border-border">
                {o.deck ? (
                  <SlideSvg deck={{ ...o.deck, theme: THEMES[theme] }} slide={o.deck.slides[0]} assetUrl={() => ""} fontsReady={fonts} className="block aspect-video w-full" label={`${o.label} preview`} />
                ) : (
                  <span aria-hidden className="block aspect-video w-full bg-surface-sunken px-2 py-1.5 font-mono text-10 leading-[1.45] text-fg-secondary">
                    • Intro
                    <br />
                    &nbsp;&nbsp;– why now
                    <br />• Plan
                  </span>
                )}
              </span>
              <span className="mt-2 block truncate text-13 font-medium text-fg">{o.label}</span>
              <span className="block text-11 text-fg-tertiary">{o.detail}</span>
              {on && (
                <span aria-hidden className="absolute right-3 top-3 grid size-4 place-items-center rounded-full bg-accent text-10 text-[#fff]">
                  ✓
                </span>
              )}
            </button>
          );
        })}
      </div>
      {starters.isError && <p className="text-12 text-fg-tertiary">Starter decks couldn’t load; you can still start blank or from an outline.</p>}
    </section>
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
        <span className="mt-3 block truncate pr-8 text-14 font-medium text-fg">{renaming ? " " : d.title}</span>
        <span className="block text-12 text-fg-tertiary">
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
            className="h-[26px] w-full rounded-md border border-accent bg-surface px-2 text-14 text-fg outline-none"
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
      {error && <p role="alert" className="mt-1 text-12 text-pill-blocked-fg">{errorText(error)}</p>}
      <Dialog.Root open={confirming} onOpenChange={setConfirming}>
        <Dialog.Portal>
          <Dialog.Overlay className="motion-scrim fixed inset-0 z-40 bg-scrim" />
          <Dialog.Content className="motion-dialog fixed left-1/2 top-[14vh] z-50 w-[min(420px,calc(100%-32px))] -translate-x-1/2 rounded-xl border border-border-float bg-surface-raised p-4 shadow-[var(--shadow-float)] outline-none">
            <Dialog.Title className="type-heading">Delete “{d.title}”?</Dialog.Title>
            <Dialog.Description className="mt-2 text-13 text-fg-secondary">The deck, its versions, its images and its chat are deleted. This can’t be undone.</Dialog.Description>
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
