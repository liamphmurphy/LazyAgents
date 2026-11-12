"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { Dialog, DropdownMenu } from "radix-ui";
import { useMemo, useState, type FormEvent, type ReactNode } from "react";
import { useCreateDesign, useDeleteDesign, useDesigns, useDesignVersion, useDuplicateDesign, useRenameDesign, type DesignStarter, type DesignSummary } from "@/lib/api/designs-hooks";
import { cx } from "@/lib/cx";
import { ago } from "@/lib/format";
import { FILTERS, briefTitle, filterDesigns, isBrief, kindOf, linksOf, type Filter } from "@/lib/designs/home";
import { DesignSvg, designAssetHref, useDesignFonts } from "@/lib/designs/render";
import { normalize, type Design } from "@/lib/designs/schema";
import { floatPanel } from "@/components/domain/pickers";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { Banner, EmptyState, Skeleton } from "@/components/ui/primitives";
import { TopBar } from "@/components/shell/topbar";

const errorText = (e: unknown) => (e instanceof Error ? e.message : "Something went wrong. Please try again.");
const menuItem = "flex cursor-pointer select-none items-center rounded-md px-2.5 py-1.5 text-[13px] text-fg outline-none data-[highlighted]:bg-hover";
const sectionLabel = "text-[11px] font-semibold uppercase tracking-[0.08em] text-fg-tertiary";

/** The Designs app's Home (docs/design/designs-home.svg): start a design and find recent ones. */
export function DesignsScreen() {
  const designs = useDesigns();
  const create = useCreateDesign();
  const router = useRouter();
  const [prompt, setPrompt] = useState("");
  const [filter, setFilter] = useState<Filter>("all");
  const [query, setQuery] = useState("");
  useDesignFonts();

  const startWith = (starter: DesignStarter, title = "") =>
    create.mutate({ starter, title }, { onSuccess: (d) => router.push(`/apps/designs/${d.id}`) });

  const start = (e?: FormEvent) => {
    e?.preventDefault();
    if (create.isPending) return;
    const text = prompt.trim();
    // A request (or a ticket key) starts a blank design and asks the persona to draft it in the design's chat.
    if (text && isBrief(text)) {
      create.mutate({ starter: "blank", title: briefTitle(text) }, { onSuccess: (d) => router.push(`/apps/designs/${d.id}?brief=${encodeURIComponent(text)}`) });
      return;
    }
    startWith("blank", text);
  };

  const list = useMemo(() => designs.data ?? [], [designs.data]);
  const shown = useMemo(() => filterDesigns(list, filter, query), [list, filter, query]);
  const review = list.filter((d) => d.pending_proposals > 0).length;

  return (
    <div className="flex h-dvh flex-col">
      <TopBar crumbs={[{ label: "Apps" }, { label: "Designs" }]} />
      <main className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto flex max-w-[1180px] flex-col gap-6 px-6 py-6 max-sm:px-4">
          <div className="flex items-start justify-between gap-4">
            <div>
              <h1 className="type-display text-fg">Designs</h1>
              <p className="mt-1 text-[13px] text-fg-secondary">App flows, diagrams and wireframes you build with your personas.</p>
            </div>
            <Button variant="primary" icon="plus" disabled={create.isPending} onClick={() => startWith("blank")}>
              New design
            </Button>
          </div>

          <form onSubmit={start} className="flex flex-col gap-3 rounded-xl border border-border-strong bg-surface p-3 focus-within:border-accent">
            <div className="flex items-center gap-2.5">
              <span aria-hidden className="grid size-[26px] flex-none place-items-center rounded-full border border-accent text-[12px] font-bold text-accent">✦</span>
              <input
                aria-label="Describe a design, or paste a ticket key"
                className="min-w-0 flex-1 bg-transparent text-[14px] text-fg outline-none placeholder:text-fg-tertiary"
                placeholder="Describe a flow or diagram, or paste a ticket key…"
                maxLength={2000}
                value={prompt}
                onChange={(e) => setPrompt(e.target.value)}
              />
            </div>
            <div className="flex flex-wrap items-center justify-between gap-2">
              <p className="text-[12px] text-fg-tertiary">A title starts a blank design; a description or ticket key opens it with its chat drafting from it.</p>
              <Button type="submit" variant="primary" disabled={create.isPending}>
                {create.isPending ? "Creating…" : "Start"}
              </Button>
            </div>
          </form>
          {create.isError && <p role="alert" className="text-[12px] text-pill-blocked-fg">{errorText(create.error)}</p>}

          <section aria-labelledby="designs-start" className="flex flex-col gap-3">
            <h2 id="designs-start" className={sectionLabel}>Start from</h2>
            <ul className="grid grid-cols-[repeat(auto-fill,minmax(168px,1fr))] gap-3">
              {STARTERS.map((s) => (
                <li key={s.key}>
                  <button
                    type="button"
                    disabled={create.isPending}
                    onClick={() => startWith(s.key)}
                    className="flex h-[128px] w-full flex-col justify-between rounded-xl border border-border bg-surface p-3 text-left outline-none transition-colors hover:border-border-hover focus-visible:border-accent focus-visible:ring-2 focus-visible:ring-accent/35 disabled:opacity-60"
                  >
                    <span aria-hidden className="flex h-[70px] items-center justify-center">{s.art}</span>
                    <span className="text-[13px] font-semibold text-fg">{s.label}</span>
                  </button>
                </li>
              ))}
              <li>
                <button
                  type="button"
                  onClick={() => {
                    setPrompt((p) => (p.trim() ? p : "Turn  into a flow"));
                    requestAnimationFrame(() => {
                      const input = document.querySelector<HTMLInputElement>('input[aria-label="Describe a design, or paste a ticket key"]');
                      input?.focus();
                      input?.setSelectionRange(5, 5);
                    });
                  }}
                  className="flex h-[128px] w-full flex-col justify-between rounded-xl border border-accent/40 bg-accent-soft/40 p-3 text-left outline-none hover:border-accent focus-visible:ring-2 focus-visible:ring-accent/35"
                >
                  <span className="flex items-center gap-1.5 text-[13px] font-semibold text-fg"><span aria-hidden className="size-2 rounded-full bg-state-review" />From a ticket</span>
                  <span className="text-[12px] text-fg-secondary">Your persona drafts the flow from its criteria</span>
                </button>
              </li>
            </ul>
          </section>

          <section aria-labelledby="designs-recent" className="flex flex-col gap-3">
            <div className="flex flex-wrap items-center gap-3">
              <h2 id="designs-recent" className={cx(sectionLabel, "mr-4")}>Recent</h2>
              <div role="tablist" aria-label="Show" className="flex items-center gap-1">
                {FILTERS.map((f) => (
                  <button
                    key={f.key}
                    type="button"
                    role="tab"
                    aria-selected={filter === f.key}
                    onClick={() => setFilter(f.key)}
                    className={cx("flex items-center gap-1.5 rounded-md px-2.5 py-1 text-[12.5px]", filter === f.key ? "bg-selected text-fg" : "text-fg-secondary hover:bg-hover")}
                  >
                    {f.label}
                    {f.key === "review" && review > 0 && <span className="rounded bg-pill-review-bg px-1.5 text-[11px] text-pill-review-fg">{review}</span>}
                  </button>
                ))}
              </div>
              <input
                type="search"
                aria-label="Search designs"
                placeholder="Search designs"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                className="ml-auto h-[30px] w-[200px] rounded-md border border-border bg-surface px-2.5 text-[12.5px] text-fg outline-none placeholder:text-fg-tertiary focus:border-accent max-sm:w-full"
              />
            </div>
            {designs.isError ? (
              <Banner tone="blocked" title="Couldn’t load your designs" actions={<Button onClick={() => void designs.refetch()}>Retry</Button>}>
                {errorText(designs.error)} Your designs are safe.
              </Banner>
            ) : !designs.data ? (
              <div aria-busy="true" className="grid grid-cols-[repeat(auto-fill,minmax(260px,1fr))] gap-4">
                {[0, 1, 2, 3].map((i) => (
                  <div key={i} className="rounded-xl border border-border bg-surface p-3">
                    <Skeleton className="h-[130px] w-full" />
                    <Skeleton className="mt-3 h-3 w-2/3" />
                    <Skeleton className="mt-2 h-3 w-1/3" />
                  </div>
                ))}
              </div>
            ) : list.length === 0 ? (
              <EmptyState icon="designs" title="No designs yet">
                Start one above. Every design you make, and every one a persona drafts for you, shows up here.
              </EmptyState>
            ) : shown.length === 0 ? (
              <p className="py-6 text-center text-[13px] text-fg-tertiary">No designs match.</p>
            ) : (
              <ul aria-label="Designs" className="grid grid-cols-[repeat(auto-fill,minmax(260px,1fr))] gap-4">
                {shown.map((d) => (
                  <DesignCard key={d.id} d={d} />
                ))}
              </ul>
            )}
          </section>

          <section aria-labelledby="designs-also" className="flex flex-col gap-3 pb-6">
            <h2 id="designs-also" className={sectionLabel}>Also in Designs</h2>
            <div className="grid grid-cols-[repeat(auto-fill,minmax(280px,1fr))] gap-3">
              <AlsoCard icon="◐" title="Token sets">Dusk (web) from tokens.css, kept in sync with a badge</AlsoCard>
              <AlsoCard icon="◇" title="Component libraries">Components you make in a design, reused across its frames</AlsoCard>
              <AlsoCard icon="⇪" title="Import">Mermaid and SVG on a page; a Figma file later</AlsoCard>
            </div>
          </section>
        </div>
      </main>
    </div>
  );
}

function AlsoCard({ icon, title, children }: { icon: string; title: string; children: ReactNode }) {
  return (
    <div className="rounded-xl border border-border bg-surface px-4 py-3">
      <p className="text-[13px] text-fg"><span aria-hidden className="mr-1.5">{icon}</span>{title}</p>
      <p className="mt-1 text-[12px] text-fg-secondary">{children}</p>
    </div>
  );
}

/** Starter cards with the mockup's little pictures. */
const STARTERS: { key: DesignStarter; label: string; art: ReactNode }[] = [
  { key: "blank", label: "Blank", art: <span className="text-[22px] font-light text-fg-tertiary">+</span> },
  {
    key: "mobile-flow",
    label: "Mobile flow",
    art: (
      <svg width="104" height="64" viewBox="0 0 104 64" fill="none">
        <rect x="10" y="2" width="30" height="60" rx="4" fill="#ECECF3" /><rect x="15" y="50" width="20" height="5" rx="2" fill="#7C6AF0" />
        <rect x="70" y="2" width="30" height="60" rx="4" fill="#ECECF3" /><path d="M40 52c14 0 14-26 28-26" stroke="#4F8EF7" strokeWidth="1.5" /><path d="m66 23 4 3-4 3z" fill="#4F8EF7" />
      </svg>
    ),
  },
  {
    key: "web-flow",
    label: "Web flow",
    art: (
      <svg width="140" height="56" viewBox="0 0 140 56" fill="none">
        <rect x="2" y="4" width="62" height="44" rx="3" fill="#ECECF3" /><rect x="2" y="4" width="62" height="7" rx="2" fill="#B9B9C8" />
        <rect x="86" y="18" width="52" height="36" rx="3" fill="#ECECF3" /><rect x="86" y="18" width="52" height="7" rx="2" fill="#B9B9C8" /><path d="M64 30c10 0 10 6 20 6" stroke="#4F8EF7" strokeWidth="1.5" />
      </svg>
    ),
  },
  {
    key: "diagram",
    label: "Diagram",
    art: (
      <svg width="148" height="64" viewBox="0 0 148 64" fill="none" stroke="#8A8AA0" strokeWidth="1.2">
        <rect x="4" y="4" width="40" height="20" rx="4" /><rect x="74" y="4" width="40" height="20" rx="4" /><rect x="74" y="42" width="40" height="20" rx="4" />
        <path d="M44 14h30M94 24v18" /><path d="m132 4 10 10-10 10-10-10z" stroke="#9D8CFF" />
      </svg>
    ),
  },
  {
    key: "wireframe",
    label: "Wireframe",
    art: (
      <svg width="140" height="60" viewBox="0 0 140 60" fill="none" stroke="#8A8AA0" strokeWidth="1.2" strokeDasharray="3 2">
        <rect x="2" y="2" width="136" height="56" rx="3" /><path d="M2 16h136M38 16v42" /><path d="M50 28h60M50 36h40" strokeDasharray="none" />
      </svg>
    ),
  },
];

/** A recent design: its first page's picture, what it is, linked tickets, suggestions waiting. */
function DesignCard({ d }: { d: DesignSummary }) {
  const router = useRouter();
  const rename = useRenameDesign();
  const duplicate = useDuplicateDesign();
  const remove = useDeleteDesign();
  const version = useDesignVersion(d.id, d.seq);
  const body = useMemo<Design | undefined>(() => {
    try {
      return version.data ? normalize(version.data.body).design : undefined;
    } catch {
      return undefined;
    }
  }, [version.data]);
  const [renaming, setRenaming] = useState(false);
  const [name, setName] = useState(d.title);
  const [confirming, setConfirming] = useState(false);
  const error = rename.error ?? duplicate.error ?? remove.error;
  const links = linksOf(body);

  const commitRename = () => {
    setRenaming(false);
    if (name.trim() && name.trim() !== d.title) rename.mutate({ id: d.id, title: name.trim() });
    else setName(d.title);
  };

  return (
    <li className="group relative" data-testid="design-card">
      <Link
        href={`/apps/designs/${d.id}`}
        className="flex h-full flex-col rounded-xl border border-border bg-surface outline-none transition-colors hover:border-border-hover focus-visible:border-accent focus-visible:ring-2 focus-visible:ring-accent/35"
      >
        <span className="block h-[140px] overflow-hidden rounded-t-xl border-b border-border-subtle bg-surface-sunken p-3 [background-image:radial-gradient(var(--color-border-subtle)_1px,transparent_1px)] [background-size:12px_12px]">
          {body ? <DesignSvg design={body} assetUrl={(a) => designAssetHref(d.id, a)} className="block h-full w-full" title={`First page of ${d.title}`} pad={8} /> : <Skeleton className="h-full w-full opacity-40" />}
        </span>
        <span className="flex flex-1 flex-col gap-1 p-3 pr-10">
          <span className="truncate text-[13.5px] font-semibold text-fg">{renaming ? " " : d.title}</span>
          <span className="text-[12px] text-fg-tertiary">
            {kindOf(body, d.frames)} · edited {ago(d.updated_at)}
          </span>
          {(links.length > 0 || d.pending_proposals > 0) && (
            <span className="mt-1 flex flex-wrap gap-1.5">
              {links.map((l) => (
                <span key={l} className="max-w-[160px] truncate rounded bg-surface-sunken px-1.5 py-0.5 font-mono text-[11px] text-fg-secondary">{l}</span>
              ))}
              {d.pending_proposals > 0 && <span className="rounded-full bg-pill-review-bg px-2 py-0.5 text-[11px] text-pill-review-fg">{d.pending_proposals} to review</span>}
            </span>
          )}
        </span>
        {d.persona_edited && <span aria-label="A persona worked on this" className="absolute bottom-3 right-11 grid size-[20px] place-items-center rounded-full border border-accent text-[10px] font-bold text-accent">✦</span>}
      </Link>
      {renaming && (
        <form
          className="absolute inset-x-3 top-[152px]"
          onSubmit={(e) => {
            e.preventDefault();
            commitRename();
          }}
        >
          <input
            autoFocus
            aria-label="Design title"
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
            className="absolute bottom-3 right-3 grid size-[26px] place-items-center rounded-md text-fg-secondary outline-none hover:bg-hover focus-visible:ring-2 focus-visible:ring-accent data-[state=open]:bg-hover"
          >
            <Icon name="dots" size={15} />
          </button>
        </DropdownMenu.Trigger>
        <DropdownMenu.Portal>
          <DropdownMenu.Content align="end" sideOffset={6} className={cx(floatPanel, "z-50 w-44")}>
            <DropdownMenu.Item className={menuItem} onSelect={() => router.push(`/apps/designs/${d.id}`)}>Open</DropdownMenu.Item>
            <DropdownMenu.Item className={menuItem} onSelect={() => setRenaming(true)}>Rename</DropdownMenu.Item>
            <DropdownMenu.Item className={menuItem} disabled={duplicate.isPending} onSelect={() => duplicate.mutate(d.id)}>Duplicate</DropdownMenu.Item>
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
            <Dialog.Description className="mt-2 text-[13px] text-fg-secondary">The design, its versions, suggestions and images are deleted. A shared chat is kept; a chat dedicated to this design is deleted. This can’t be undone.</Dialog.Description>
            <div className="mt-4 flex justify-end gap-2">
              <Dialog.Close asChild>
                <Button variant="ghost">Cancel</Button>
              </Dialog.Close>
              <Button variant="primary" className="bg-state-blocked text-[#fff]" disabled={remove.isPending} onClick={() => remove.mutate(d.id, { onSuccess: () => setConfirming(false) })}>
                {remove.isPending ? "Deleting…" : "Delete design"}
              </Button>
            </div>
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>
    </li>
  );
}
