"use client";

import { Dialog } from "radix-ui";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { hasLiveRun, useArtifact, useAskChat, useBootstrap, useChats, useControlRun, useCreateChat, useDecideApproval, useInbox, useLiveRuns, useSearch, useTicket, useUpdateTicket, useWikiPage } from "@/lib/api/hooks";
import { IconButton } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { Kbd } from "@/components/ui/primitives";
import { artifactTitle } from "@/components/domain/artifacts";
import { cx } from "@/lib/cx";
import { attachQuery, type ContextRef } from "@/lib/chat-context";
import {
  linkFromTarget,
  matchParts,
  matchesWords,
  pageContext,
  parseFilters,
  parseRecent,
  passesFilters,
  promoteTop,
  projectOfKey,
  pushRecent,
  removeToken,
  snippetParts,
  sortByGroup,
  ticketKeyFrom,
  type PaletteGroup,
  type PaletteLink,
} from "@/lib/command-palette";
import { NAV_GROUPS, projectSection, switchHref, withProject } from "@/lib/project-scope";
import { rememberProject, useRememberedProject } from "./project-scope";

const RECENT_KEY = "la:palette-recent";

const GLYPH: Partial<Record<PaletteGroup, string>> = {
  Tickets: "◇",
  Pages: "▤",
  Artifacts: "▦",
  Chats: "◌",
  Agents: "◍",
  Epics: "◆",
  Runs: "▶",
  Workflows: "⇢",
  Personas: "☺",
  Memories: "✧",
  Library: "▣",
  "Needs you": "!",
  Running: "▶",
  Continue: "◌",
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
  snippet?: string;
  score?: number;
  /** Takes the query as its input, so it stays offered whatever was typed. */
  fallback?: boolean;
  /** Only offered once asked for: too specific, or too consequential, for the opening list. */
  onDemand?: boolean;
};

const MARK = "rounded-[3px] bg-accent-soft text-fg underline decoration-accent decoration-2 underline-offset-4";

const ROOT_KEYS = [["↑↓", "navigate"], ["↵", "open"], ["⌘↵", "new tab"], ["Tab", "project scope"], [">", "commands"], ["is: status:", "filters"]] as const;
const PROJECT_KEYS = [["↑↓", "navigate"], ["↵", "switch"], ["⌫", "back"], ["Esc", "close"]] as const;

/** A single-key shortcut ("C", "?") reads as a key; anything longer is context. */
const isShortcut = (meta: string) => [...meta].length <= 2;
const STATUSES = [
  ["backlog", "Backlog"],
  ["todo", "Todo"],
  ["in_progress", "In progress"],
  ["in_review", "In review"],
  ["done", "Done"],
] as const;

function Parts({ parts }: { parts: { text: string; match: boolean }[] }) {
  return (
    <>
      {parts.map((p, i) => (p.match ? <span key={i} className={MARK}>{p.text}</span> : <span key={i}>{p.text}</span>))}
    </>
  );
}

function Match({ text, query }: { text: string; query: string }) {
  return <Parts parts={matchParts(text, query)} />;
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
  const askChat = useAskChat();
  const remembered = useRememberedProject();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [project, setProject] = useState<string>();
  const [scope, setScope] = useState(false);
  const [mode, setMode] = useState<"root" | "projects">("root");
  const [active, setActive] = useState(0);
  const [recent, setRecent] = useState<PaletteLink[]>([]);
  const [actionFailed, setActionFailed] = useState(false);
  const returnFocus = useRef<HTMLElement | null>(null);
  const rememberedRef = useRef(remembered);
  const inputRef = useRef<HTMLInputElement>(null);

  const commands = query.startsWith(">");
  const filters = parseFilters(commands ? "" : query);
  const title = commands ? query.replace(/^>\s*/, "").trim() : filters.text;
  const searchText = mode === "root" && !commands ? filters.text : "";
  const text = useDebounced(searchText, 150);
  const search = useSearch(open ? text : "", scope ? project : undefined);
  // Until the answer for exactly this text arrives, the rows on screen are stale.
  const pending = !!searchText && (text !== searchText || search.isLoading || search.isPlaceholderData);

  // What the page is about (only fetched while open; the page usually has it cached).
  const ctx = pageContext(pathname);
  const ticket = useTicket(open && ctx?.kind === "ticket" ? ctx.key : "");
  const artifact = useArtifact(open && ctx?.kind === "artifact" ? ctx.id : undefined);
  const wiki = useWikiPage(open && ctx?.kind === "wiki" ? ctx.path : "");
  const ticketKey = ctx?.kind === "ticket" ? ctx.key : "";
  const updateTicket = useUpdateTicket(ticketKey);
  const controlRun = useControlRun(ticketKey);
  const decideApproval = useDecideApproval(ticketKey);
  let attach: { ref: ContextRef; label: string; project?: string } | undefined;
  if (ctx?.kind === "ticket") attach = { ref: { kind: "ticket", key: ctx.key }, label: ctx.key, project: projectOfKey(ctx.key) };
  const doc = ctx?.kind === "artifact" ? artifact.data : ctx?.kind === "wiki" ? wiki.data : undefined;
  if (doc) attach = { ref: { kind: doc.type === "note" ? "page" : "artifact", id: doc.id }, label: artifactTitle(doc), project: doc.project?.key };

  const projects = (boot.data?.projects ?? []).filter((p) => !p.archived);
  // Where actions happen: the page's project, then the URL's, then the remembered one.
  const actionProject = attach?.project ?? project ?? remembered ?? projects[0]?.key;

  // The opening list: what's waiting on you, what's moving, where you left off.
  const idle = open && mode === "root" && !query.trim();
  const inbox = useInbox(undefined, idle);
  const liveRuns = useLiveRuns(idle);
  const chats = useChats(false, "", idle, "all");

  const close = (next: boolean) => {
    // A failed action's alert belongs to that attempt only.
    if (!next) {
      createChat.reset();
      askChat.reset();
      setActionFailed(false);
    }
    setOpen(next);
  };

  const failed = () => {
    setActionFailed(true);
    setOpen(true);
  };

  const go = (item: Item, newTab = false) => {
    if (newTab && item.href) {
      window.open(item.href, "_blank", "noopener");
      return;
    }
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

  const key = mode === "root" && !commands ? ticketKeyFrom(query, actionProject) : undefined;
  const jump: Item[] = key
    ? [{ id: `jump-${key}`, title: `Open ${key}`, meta: `/tickets/${key}`, group: "Jump to", href: `/tickets/${key}`,
        recent: { id: `jump-${key}`, title: key, meta: key, href: `/tickets/${key}`, kind: "Tickets" } }]
    : [];

  const hits: Item[] = searchText
    ? promoteTop(
        sortByGroup(
          (search.data ?? []).flatMap((h) => {
            const link = linkFromTarget(h.target);
            if (!link) return [];
            const group = link.kind as PaletteGroup;
            if (!passesFilters({ group, status: link.status }, filters)) return [];
            const meta = [link.meta, link.status?.replaceAll("_", " ")].filter(Boolean).join(" · ");
            return [{ id: link.id, title: link.title, meta, group, glyph: GLYPH[group], href: link.href, recent: link, snippet: h.snippet, score: h.score }];
          }),
        ),
      )
    : [];

  const recentItems: Item[] =
    mode === "root" && !query.trim()
      ? recent.map((r) => ({ id: `recent-${r.href}`, title: r.title, meta: r.meta, group: "Recent", glyph: GLYPH[r.kind as PaletteGroup], href: r.href, recent: r }))
      : [];

  const needsYou: Item[] = idle
    ? (inbox.data ?? []).filter((i) => !i.resolved).slice(0, 3).map((i) => ({ id: `inbox-${i.id}`, title: i.title, meta: i.ticket_key || i.project, group: "Needs you" as const, href: i.href }))
    : [];
  const running: Item[] = idle
    ? (liveRuns.data?.runs ?? []).slice(0, 3).map((r) => ({ id: `live-${r.id}`, title: r.ticket_key ?? r.agent.name, meta: r.state, group: "Running" as const, href: `/runs/${r.id}` }))
    : [];
  const lastChat = idle ? chats.data?.chats[0] : undefined;
  const resume: Item[] = lastChat
    ? [{ id: `continue-${lastChat.id}`, title: lastChat.title || "Untitled chat", meta: "last chat", group: "Continue", href: `/chat/${lastChat.id}` }]
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
      fallback: true,
      href: withQuery("/tickets/new", { title: title || undefined, project: actionProject }),
    },
    ...(title && actionProject
      ? [{
          id: "ask",
          title: `Ask: ${title}`,
          meta: "new chat",
          glyph: "✉",
          group: "Actions" as const,
          fallback: true,
          perform: () =>
            askChat.mutate(
              { project: actionProject, body: title },
              { onSuccess: (c) => router.push(`/chat/${c.id}`), onError: () => setOpen(true) },
            ),
        }]
      : []),
    {
      id: "new-chat",
      title: attach ? `New chat with ${attach.label} attached` : "New chat",
      glyph: "◌",
      group: "Actions",
      perform: () => {
        const p = attach?.project;
        createChat.mutate(
          p ? { project: p } : {},
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
      fallback: true,
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
    ...(ctx?.kind === "ticket" && ticket.data && hasLiveRun(ticket.data) && ticket.data.run
      ? [{
          id: "cancel-run",
          title: `Cancel run for ${ctx.key}`,
          glyph: "■",
          group: "Actions" as const,
          onDemand: true,
          perform: () => controlRun.mutate({ id: ticket.data!.run!.id, action: "cancel" }, { onError: failed }),
        }]
      : []),
    ...(ticket.data && ctx?.kind === "ticket"
      ? STATUSES.filter(([s]) => s !== ticket.data!.status).map(([status, label]) => ({
          id: `status-${status}`,
          title: `Set status: ${label}`,
          glyph: "◇",
          group: "Actions" as const,
          onDemand: true,
          perform: () => updateTicket.mutate({ status, revision: ticket.data!.revision }, { onError: failed }),
        }))
      : []),
    ...(ticket.data && ctx?.kind === "ticket"
      ? ticket.data.approvals.filter((a) => a.state === "pending").map((a) => ({
          id: `approve-${a.id}`,
          title: `Approve: ${a.action_summary}`,
          glyph: "✓",
          group: "Actions" as const,
          onDemand: true,
          perform: () => decideApproval.mutate({ id: a.id, decision: "approve" }, { onError: failed }),
        }))
      : []),
    {
      id: "copy-link",
      title: "Copy link to this page",
      glyph: "⧉",
      group: "Actions",
      onDemand: true,
      perform: () => void navigator.clipboard.writeText(window.location.href).catch(failed),
    },
    {
      id: "shortcuts",
      title: "Keyboard shortcuts",
      meta: "?",
      glyph: "⌨",
      group: "Actions",
      perform: () => window.dispatchEvent(new Event("la:shortcuts")),
    },
  ];
  const shownActions = actions.filter((a) => {
    if (commands) return matchesWords(a.title, title);
    if (title) return a.fallback || matchesWords(a.title, title);
    return !a.onDemand;
  });

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
      : [...needsYou, ...running, ...resume, ...recentItems, ...jump, ...hits, ...shownActions, ...(commands && title ? projectItems : []), ...goTo];
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
    rememberedRef.current = remembered;
  }, [remembered]);

  useEffect(() => {
    const show = () => {
      if (document.activeElement instanceof HTMLElement && document.activeElement !== document.body)
        returnFocus.current = document.activeElement;
      else returnFocus.current = null;
      const currentProject =
        new URLSearchParams(window.location.search).get("project") ??
        window.location.pathname.match(/^\/tickets\/([A-Z][A-Z0-9]*)-\d+$/)?.[1] ??
        // A chat's URL may not name its project; the remembered one is what actions use too.
        (projectSection(window.location.pathname) ? rememberedRef.current : undefined);
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
        <Dialog.Overlay className="motion-scrim fixed inset-0 z-50 bg-scrim backdrop-blur-[3px]" />
        <Dialog.Content
          className="motion-dialog-fast fixed left-1/2 top-[15vh] z-50 flex max-h-[70vh] w-[640px] max-w-[calc(100vw-24px)] -translate-x-1/2 flex-col overflow-hidden rounded-2xl border border-border-float bg-surface-raised shadow-2xl"
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
          {/* A hairline of the theme's accent along the top edge: the palette's only ornament. */}
          <div aria-hidden className="pointer-events-none absolute inset-x-8 top-0 h-px bg-gradient-to-r from-transparent via-accent to-transparent opacity-70" />
          <Dialog.Title className="sr-only">{mode === "projects" ? "Switch project" : "Search and commands"}</Dialog.Title>
          <div className="flex items-center gap-2.5 border-b border-border px-4 py-3.5">
            <Icon name="search" size={16} className="flex-none text-fg-tertiary" />
            {mode === "projects" ? (
              <button
                type="button"
                className="rounded-full border border-border px-2.5 py-0.5 text-[11.5px] font-medium text-fg-secondary transition-colors hover:bg-hover"
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
                  className={cx(
                    "rounded-full border px-2.5 py-0.5 text-[11.5px] font-medium transition-colors",
                    scope ? "border-transparent bg-accent-soft text-accent" : "border-border text-fg-secondary hover:bg-hover",
                  )}
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
              placeholder={mode === "projects" ? "Switch to project…" : "Search, or try is:ticket status:done…"}
              className="min-w-0 flex-1 bg-transparent text-[15px] outline-none placeholder:text-fg-tertiary"
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
                // While results are stale, ↵ must not fall through to an action or an old hit.
                if (e.key === "Enter" && items[index] && (!pending || items[index].group === "Jump to")) {
                  e.preventDefault();
                  go(items[index], e.metaKey || e.ctrlKey);
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
            <span role="status" className="flex items-center gap-1.5 text-xs text-fg-tertiary">
              {pending && <span aria-hidden className="size-1.5 animate-skeleton rounded-full bg-accent" />}
              {pending ? "Searching…" : ""}
            </span>
            <span aria-hidden>
              <Kbd>esc</Kbd>
            </span>
            <Dialog.Close asChild>
              <IconButton icon="x" label="Close search" />
            </Dialog.Close>
          </div>
          {mode === "root" && filters.tokens.length > 0 && (
            <div className="flex flex-wrap gap-1 border-b border-border px-4 py-2">
              {filters.tokens.map((token) => (
                <button
                  key={token}
                  type="button"
                  aria-label={`Remove filter ${token}`}
                  className="rounded-full border border-transparent bg-accent-soft px-2.5 py-0.5 font-mono text-xs text-accent transition-colors hover:border-accent"
                  onClick={() => {
                    setQuery(removeToken(query, token));
                    inputRef.current?.focus();
                  }}
                >
                  {token} ×
                </button>
              ))}
            </div>
          )}
          <div className="scroll-py-2 overflow-y-auto p-2">
            {search.isError && (
              <p role="alert" className="p-3 text-xs text-pill-blocked-fg">
                Search failed. Try again.
              </p>
            )}
            {(createChat.isError || askChat.isError) && (
              <p role="alert" className="p-3 text-xs text-pill-blocked-fg">
                Couldn’t start the chat.
              </p>
            )}
            {actionFailed && (
              <p role="alert" className="p-3 text-xs text-pill-blocked-fg">
                That action didn’t go through. Try again.
              </p>
            )}
            {searchText && !pending && hits.length === 0 && !key && (
              <p className="px-3 py-8 text-center text-sm text-fg-tertiary">No results for “{query.trim()}”.</p>
            )}
            {mode === "projects" && items.length === 0 && (
              <p className="px-3 py-8 text-center text-sm text-fg-tertiary">No project matches “{query}”.</p>
            )}
            <div id="command-results" role="listbox" aria-label="Results" aria-busy={pending} className={cx(pending && "opacity-60")}>
              {groups.map((g) => (
                <div key={g.name} role="group" aria-labelledby={`command-group-${g.name.replace(/\s+/g, "-")}`}>
                  <div
                    id={`command-group-${g.name.replace(/\s+/g, "-")}`}
                    role="presentation"
                    className="px-3 pb-1.5 pt-3 text-[11px] font-medium tracking-wide text-fg-tertiary"
                  >
                    {g.name}
                  </div>
                  {g.items.map(({ item, i }) => (
                    <div
                      key={`${item.group}-${item.id}`}
                      id={`command-${i}`}
                      role="option"
                      aria-selected={i === index}
                      onClick={(e) => go(item, e.metaKey || e.ctrlKey)}
                      onMouseMove={() => setActive(i)}
                      className={cx(
                        "relative flex min-h-[40px] w-full cursor-pointer items-center gap-3 rounded-lg px-3 text-left text-sm transition-colors duration-100",
                        i === index ? "bg-selected text-fg" : "text-fg-secondary hover:bg-hover",
                      )}
                    >
                      {i === index && <span aria-hidden className="absolute bottom-2 left-0 top-2 w-[3px] rounded-full bg-accent" />}
                      <span
                        aria-hidden
                        className={cx(
                          "grid size-6 flex-none place-items-center rounded-md border text-[12px] transition-colors duration-100",
                          i === index ? "border-transparent bg-accent-soft text-accent" : "border-border-subtle bg-surface-sunken text-fg-tertiary",
                        )}
                      >
                        {item.glyph ?? GLYPH[item.group] ?? ""}
                      </span>
                      <span className="min-w-0 flex-1 truncate">
                        <span className="block truncate">
                          <Match text={item.title} query={mode === "projects" ? query : title} />
                        </span>
                        {item.snippet && (
                          <span className="block truncate text-xs text-fg-tertiary">
                            <Parts parts={snippetParts(item.snippet)} />
                          </span>
                        )}
                      </span>
                      {item.meta &&
                        (isShortcut(item.meta) ? (
                          <Kbd>{item.meta}</Kbd>
                        ) : (
                          <span className="max-w-[40%] flex-none truncate font-mono text-[11.5px] text-fg-tertiary">{item.meta}</span>
                        ))}
                    </div>
                  ))}
                </div>
              ))}
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-x-3.5 gap-y-1 border-t border-border bg-surface-sunken px-4 py-2 text-[11.5px] text-fg-tertiary">
            {(mode === "projects" ? PROJECT_KEYS : ROOT_KEYS).map(([keys, what]) => (
              <span key={what} className="flex items-center gap-1.5">
                <Kbd>{keys}</Kbd>
                {what}
              </span>
            ))}
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
