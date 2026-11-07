"use client";

import { Dialog } from "radix-ui";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { useArtifact, useBootstrap, useCreateChat, useSearch, useTicket, useWikiPage } from "@/lib/api/hooks";
import { IconButton } from "@/components/ui/button";
import { artifactTitle } from "@/components/domain/artifacts";
import { cx } from "@/lib/cx";
import { attachQuery, type ContextRef } from "@/lib/chat-context";
import {
  linkFromTarget,
  matchesWords,
  pageContext,
  parseRecent,
  projectOfKey,
  pushRecent,
  sortByGroup,
  ticketKeyFrom,
  type PaletteGroup,
  type PaletteLink,
} from "@/lib/command-palette";
import { NAV_GROUPS, switchHref, withProject } from "@/lib/project-scope";
import { rememberProject, useRememberedProject } from "./project-scope";

const RECENT_KEY = "la:palette-recent";

const GLYPH: Partial<Record<PaletteGroup, string>> = {
  Tickets: "◇",
  Pages: "▤",
  Artifacts: "▦",
  Chats: "◌",
  Agents: "◍",
  Epics: "◆",
  "Jump to": "↵",
  "Switch project": "⇄",
};

type Item = {
  id: string;
  title: string;
  meta?: string;
  group: PaletteGroup;
  glyph?: string;
  /** Navigates (and is remembered as recent when `recent` is set). */
  href?: string;
  recent?: PaletteLink;
  perform?: () => void;
};

function Match({ text, query }: { text: string; query: string }) {
  const word = query.trim().split(/\s+/)[0] ?? "";
  const at = word ? text.toLowerCase().indexOf(word.toLowerCase()) : -1;
  return at < 0 ? (
    <>{text}</>
  ) : (
    <>
      {text.slice(0, at)}
      <span className="underline decoration-accent underline-offset-4">{text.slice(at, at + word.length)}</span>
      {text.slice(at + word.length)}
    </>
  );
}

/** Waits for typing to pause before searching (the server ranks; nothing is filtered client-side). */
function useDebounced(value: string, ms: number) {
  const [v, setV] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => setV(value), ms);
    return () => clearTimeout(t);
  }, [value, ms]);
  return v;
}

export function CommandMenu() {
  const router = useRouter();
  const pathname = usePathname();
  const boot = useBootstrap();
  const createChat = useCreateChat();
  const remembered = useRememberedProject();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [project, setProject] = useState<string>();
  const [scope, setScope] = useState(false);
  const [mode, setMode] = useState<"root" | "projects">("root");
  const [active, setActive] = useState(0);
  const [recent, setRecent] = useState<PaletteLink[]>([]);
  const returnFocus = useRef<HTMLElement | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const commands = query.startsWith(">");
  const title = query.replace(/^>\s*/, "").trim();
  const text = useDebounced(mode === "root" && !commands ? query.trim() : "", 150);
  const search = useSearch(open ? text : "", scope ? project : undefined);

  // What the page is about (only fetched while open; the page usually has it cached).
  const ctx = pageContext(pathname);
  const ticket = useTicket(open && ctx?.kind === "ticket" ? ctx.key : "");
  const artifact = useArtifact(open && ctx?.kind === "artifact" ? ctx.id : undefined);
  const wiki = useWikiPage(open && ctx?.kind === "wiki" ? ctx.path : "");
  let attach: { ref: ContextRef; label: string; project?: string } | undefined;
  if (ctx?.kind === "ticket") attach = { ref: { kind: "ticket", key: ctx.key }, label: ctx.key, project: projectOfKey(ctx.key) };
  const doc = ctx?.kind === "artifact" ? artifact.data : ctx?.kind === "wiki" ? wiki.data : undefined;
  if (doc) attach = { ref: { kind: doc.type === "note" ? "page" : "artifact", id: doc.id }, label: artifactTitle(doc), project: doc.project?.key };

  const projects = (boot.data?.projects ?? []).filter((p) => !p.archived);
  // Where actions happen: the page's project, then the URL's, then the remembered one.
  const actionProject = attach?.project ?? project ?? remembered ?? projects[0]?.key;

  const close = (next: boolean) => {
    // A failed "New chat" alert belongs to that attempt only.
    if (!next) createChat.reset();
    setOpen(next);
  };

  const go = (item: Item) => {
    close(false);
    if (item.recent) {
      const next = pushRecent(recent, item.recent);
      setRecent(next);
      try {
        window.localStorage.setItem(RECENT_KEY, JSON.stringify(next));
      } catch {
        /* storage full or disabled: recents are a nicety */
      }
    }
    if (item.perform) item.perform();
    else if (item.href) router.push(item.href);
  };

  const switchTo = (key: string) => {
    rememberProject(key);
    const href = switchHref(window.location.pathname, new URLSearchParams(window.location.search), key);
    if (href) router.push(href);
  };

  const projectItems: Item[] = projects
    .map((p) => ({
      id: `project-${p.key}`,
      title: `${p.key} · ${p.name}`,
      meta: p.key === actionProject ? "current" : undefined,
      group: "Switch project" as const,
      perform: () => switchTo(p.key),
    }))
    .filter((i) => matchesWords(i.title, mode === "projects" ? query : title.replace(/^(switch|project)\b\s*/i, "")));

  const key = mode === "root" && !commands ? ticketKeyFrom(query) : undefined;
  const jump: Item[] = key
    ? [{ id: `jump-${key}`, title: `Open ${key}`, meta: `/tickets/${key}`, group: "Jump to", href: `/tickets/${key}`,
        recent: { id: `jump-${key}`, title: key, meta: key, href: `/tickets/${key}`, kind: "Tickets" } }]
    : [];

  const hits: Item[] =
    // Only once the debounce settles, so ↵ never opens a hit for an earlier query.
    mode === "root" && !commands && query.trim() && text === query.trim()
      ? sortByGroup(
          (search.data ?? []).flatMap((h) => {
            const link = linkFromTarget(h.target);
            if (!link) return [];
            const group = link.kind as PaletteGroup;
            return [{ id: link.id, title: link.title, meta: link.meta, group, href: link.href, recent: link }];
          }),
        )
      : [];

  const recentItems: Item[] =
    mode === "root" && !query.trim()
      ? recent.map((r) => ({ id: `recent-${r.href}`, title: r.title, meta: r.meta, group: "Recent", glyph: GLYPH[r.kind as PaletteGroup], href: r.href, recent: r }))
      : [];

  const withQuery = (base: string, params: Record<string, string | undefined>) => {
    const q = new URLSearchParams();
    for (const [k, v] of Object.entries(params)) if (v) q.set(k, v);
    const s = q.toString();
    return s ? `${base}?${s}` : base;
  };
  const actions: Item[] = [
    {
      id: "new-ticket",
      title: title ? `New ticket: ${title}` : "New ticket",
      meta: "C",
      glyph: "＋",
      group: "Actions",
      href: withQuery("/tickets/new", { title: title || undefined, project: actionProject }),
    },
    {
      id: "new-chat",
      title: attach ? `New chat with ${attach.label} attached` : "New chat",
      glyph: "◌",
      group: "Actions",
      perform: () => {
        const p = attach?.project ?? actionProject;
        if (!p) return router.push("/chat");
        createChat.mutate(
          { project: p },
          {
            onSuccess: (c) => router.push(attach ? `/chat/${c.id}?${attachQuery(p, attach.ref, attach.label)}` : `/chat/${c.id}`),
            // Reopen so the alert below explains what happened.
            onError: () => setOpen(true),
          },
        );
      },
    },
    {
      id: "plan",
      title: title ? `Start planning: ${title}` : "Start planning",
      glyph: "✦",
      group: "Actions",
      href: withQuery("/plans", {
        project: actionProject,
        goal: title || undefined,
        artifact: doc && doc.type !== "note" ? doc.id : undefined,
      }),
    },
    {
      id: "switch",
      title: "Switch project…",
      glyph: "⇄",
      group: "Actions",
      perform: () => {
        // Stay open on the project list instead of navigating.
        setMode("projects");
        setQuery("");
        setActive(0);
        setOpen(true);
      },
    },
    ...(ctx?.kind === "ticket" && ticket.data?.run
      ? [{ id: "run", title: `Go to run for ${ctx.key}`, glyph: "▶", group: "Actions" as const, meta: ticket.data.run.state, href: `/runs/${ticket.data.run.id}` }]
      : []),
    {
      id: "shortcuts",
      title: "Keyboard shortcuts",
      meta: "?",
      glyph: "⌨",
      group: "Actions",
      perform: () => window.dispatchEvent(new Event("la:shortcuts")),
    },
  ];
  const shownActions = commands ? actions.filter((a) => matchesWords(a.title, title)) : actions;

  // Same item set as the sidebar (lib/project-scope.ts), scoped to the remembered project.
  const goTo: Item[] = NAV_GROUPS.flatMap((g) =>
    g.items.map((i) => ({
      id: `nav-${i.href}`,
      title: `Go to ${i.label}`,
      meta: g.label,
      href: withProject(i.href, remembered),
      group: "Go to" as const,
    })),
  ).filter((i) => (title ? matchesWords(i.title, title) : commands));

  const items: Item[] =
    mode === "projects"
      ? projectItems
      : [...recentItems, ...jump, ...hits, ...shownActions, ...(commands && title ? projectItems : []), ...goTo];
  const index = Math.max(0, Math.min(active, items.length - 1));
  const activeId = items.length ? `command-${index}` : undefined;

  // Entering or leaving the project list keeps typing in the input.
  useEffect(() => {
    if (open) inputRef.current?.focus();
  }, [mode, open]);

  useEffect(() => {
    if (open && activeId) document.getElementById(activeId)?.scrollIntoView({ block: "nearest" });
  }, [activeId, open]);

  useEffect(() => {
    const show = () => {
      if (document.activeElement instanceof HTMLElement && document.activeElement !== document.body)
        returnFocus.current = document.activeElement;
      else returnFocus.current = null;
      const currentProject =
        new URLSearchParams(window.location.search).get("project") ??
        window.location.pathname.match(/^\/tickets\/([A-Z][A-Z0-9]*)-\d+$/)?.[1];
      setProject(currentProject ?? undefined);
      setScope(!!currentProject);
      setQuery("");
      setMode("root");
      setActive(0);
      let stored: string | null = null;
      try {
        stored = window.localStorage.getItem(RECENT_KEY);
      } catch {
        /* storage blocked: no recents */
      }
      setRecent(parseRecent(stored));
      setOpen(true);
    };
    const key = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && !e.altKey && e.key.toLowerCase() === "k") {
        e.preventDefault();
        show();
      }
    };
    window.addEventListener("keydown", key);
    window.addEventListener("la:search", show);
    return () => {
      window.removeEventListener("keydown", key);
      window.removeEventListener("la:search", show);
    };
  }, []);

  // Groups render as role=group inside the listbox; options keep one flat index for ↑↓.
  const groups: { name: PaletteGroup; items: { item: Item; i: number }[] }[] = [];
  items.forEach((item, i) => {
    const last = groups[groups.length - 1];
    if (last?.name === item.group) last.items.push({ item, i });
    else groups.push({ name: item.group, items: [{ item, i }] });
  });

  return (
    <Dialog.Root open={open} onOpenChange={close}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-50 bg-scrim" />
        <Dialog.Content
          className="fixed left-1/2 top-[15vh] z-50 flex max-h-[70vh] w-[640px] max-w-[calc(100vw-24px)] -translate-x-1/2 flex-col overflow-hidden rounded-xl border border-border-float bg-surface-raised shadow-xl"
          aria-describedby={undefined}
          onOpenAutoFocus={(e) => {
            // The input, not the scope toggle that comes first.
            e.preventDefault();
            inputRef.current?.focus();
          }}
          onCloseAutoFocus={(e) => {
            // Back to whatever had focus before ⌘K (Radix would pick the body when opened by keyboard).
            const el = returnFocus.current;
            if (el?.isConnected) {
              e.preventDefault();
              el.focus();
            }
          }}
        >
          <Dialog.Title className="sr-only">{mode === "projects" ? "Switch project" : "Search and commands"}</Dialog.Title>
          <div className="flex items-center gap-2 border-b border-border px-4 py-3">
            {mode === "projects" ? (
              <button
                type="button"
                className="rounded border border-border px-2 text-xs text-fg-secondary"
                onClick={() => {
                  setMode("root");
                  setQuery("");
                }}
              >
                ← Back
              </button>
            ) : (
              project && (
                <button
                  type="button"
                  aria-pressed={scope}
                  aria-label={scope ? `Searching ${project} only; search all projects` : `Searching all projects; search ${project} only`}
                  className="rounded border border-border px-2 text-xs text-fg-secondary"
                  onClick={() => setScope(!scope)}
                >
                  {scope ? project : "All projects"}
                </button>
              )
            )}
            <input
              ref={inputRef}
              autoComplete="off"
              spellCheck={false}
              role="combobox"
              aria-label={mode === "projects" ? "Switch to project" : "Search everything"}
              aria-expanded={items.length > 0}
              aria-autocomplete="list"
              aria-controls="command-results"
              aria-activedescendant={activeId}
              value={query}
              onChange={(e) => {
                setQuery(e.target.value);
                setActive(0);
              }}
              placeholder={mode === "projects" ? "Switch to project…" : "Search or jump to a ticket key…"}
              className="min-w-0 flex-1 bg-transparent text-sm outline-none"
              onKeyDown={(e) => {
                if (e.key === "ArrowDown" && items.length) {
                  e.preventDefault();
                  setActive((index + 1) % items.length);
                }
                if (e.key === "ArrowUp" && items.length) {
                  e.preventDefault();
                  setActive((index + items.length - 1) % items.length);
                }
                if (e.key === "Home" && items.length) {
                  e.preventDefault();
                  setActive(0);
                }
                if (e.key === "End" && items.length) {
                  e.preventDefault();
                  setActive(items.length - 1);
                }
                if (e.key === "Enter" && items[index]) {
                  e.preventDefault();
                  go(items[index]);
                }
                if (e.key === "Backspace" && mode === "projects" && !query) {
                  e.preventDefault();
                  setMode("root");
                }
                if (e.key === "Tab" && project && mode === "root") {
                  e.preventDefault();
                  setScope(!scope);
                }
              }}
            />
            <span aria-hidden className="font-mono text-[11px] text-fg-tertiary">esc</span>
            <Dialog.Close asChild>
              <IconButton icon="x" label="Close search" />
            </Dialog.Close>
          </div>
          <div className="overflow-y-auto p-2">
            {search.isFetching && (
              <p role="status" className="p-3 text-xs text-fg-tertiary">
                Searching…
              </p>
            )}
            {search.isError && (
              <p role="alert" className="p-3 text-xs text-pill-blocked-fg">
                Search failed. Try again.
              </p>
            )}
            {createChat.isError && (
              <p role="alert" className="p-3 text-xs text-pill-blocked-fg">
                Couldn’t start the chat.
              </p>
            )}
            {mode === "root" && !commands && query.trim() && text === query.trim() && !search.isFetching && hits.length === 0 && !key && (
              <p className="p-3 text-sm text-fg-tertiary">No results for “{query.trim()}”.</p>
            )}
            {mode === "projects" && items.length === 0 && (
              <p className="p-3 text-sm text-fg-tertiary">No project matches “{query}”.</p>
            )}
            <div id="command-results" role="listbox" aria-label="Results">
              {groups.map((g) => (
                <div key={g.name} role="group" aria-labelledby={`command-group-${g.name.replace(/\s+/g, "-")}`}>
                  <div
                    id={`command-group-${g.name.replace(/\s+/g, "-")}`}
                    role="presentation"
                    className="px-3 pb-1 pt-3 text-xs text-fg-tertiary"
                  >
                    {g.name}
                  </div>
                  {g.items.map(({ item, i }) => (
                    <div
                      key={`${item.group}-${item.id}`}
                      id={`command-${i}`}
                      role="option"
                      aria-selected={i === index}
                      onClick={() => go(item)}
                      onMouseMove={() => setActive(i)}
                      className={cx(
                        "flex min-h-[38px] w-full cursor-pointer items-center gap-3 rounded-md px-3 text-left text-sm",
                        i === index ? "bg-selected" : "hover:bg-hover",
                      )}
                    >
                      <span aria-hidden className="w-4 flex-none text-center text-fg-tertiary">
                        {item.glyph ?? GLYPH[item.group] ?? ""}
                      </span>
                      <span className="min-w-0 flex-1 truncate">
                        <Match text={item.title} query={mode === "projects" ? query : title} />
                      </span>
                      {item.meta && <span className="font-mono text-xs text-fg-tertiary">{item.meta}</span>}
                    </div>
                  ))}
                </div>
              ))}
            </div>
          </div>
          <div className="border-t border-border px-4 py-2 text-xs text-fg-tertiary">
            {mode === "projects"
              ? "↑↓ navigate · ↵ switch · ⌫ back · Esc close"
              : "↑↓ navigate · ↵ open · Tab project scope · > commands only · ? shortcuts"}
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
