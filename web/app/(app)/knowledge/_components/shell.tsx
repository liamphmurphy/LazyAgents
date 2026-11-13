"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { createContext, useCallback, useContext, useEffect, useMemo, useState, useSyncExternalStore, type ReactNode } from "react";
import { claimEditorRail } from "@/lib/editor-layout";
import { cx } from "@/lib/cx";
import { ancestors, buildTree, closeTab, isoDay, openTab, pageHref, type Tab, type TreeFolder, type TreeRow } from "@/lib/knowledge";
import { useAllPages, useCreatePage, useDaily, usePage } from "@/lib/api/knowledge-hooks";
import { Icon, type IconName } from "@/components/ui/icon";
import { Kbd, Skeleton } from "@/components/ui/primitives";
import { isTyping } from "@/components/shell/shortcuts";
import { QuickSwitcher } from "./switcher";
import { CaptureDialog } from "./capture";
import { ImportDialog } from "./import-dialog";

type View = "yours" | "agent";

type Shell = {
  view: View;
  setView: (v: View) => void;
  openSwitcher: () => void;
  openCapture: () => void;
  openImport: () => void;
  openToday: () => void;
  /** Registers the screen showing now as a tab. */
  registerTab: (tab: Tab) => void;
};

const Ctx = createContext<Shell | null>(null);

export function useKnowledge(): Shell {
  const s = useContext(Ctx);
  if (!s) throw new Error("useKnowledge outside KnowledgeShell");
  return s;
}

/** Registers the current screen as a tab, and keeps its title current as it loads or is renamed. */
export function useKnowledgeTab(tab: Tab | undefined) {
  const { registerTab } = useKnowledge();
  const id = tab?.id;
  const title = tab?.title;
  const kind = tab?.kind;
  useEffect(() => {
    if (id) registerTab({ id, title: title ?? "", kind });
  }, [registerTab, id, title, kind]);
}

const TABS_KEY = "vesper.knowledge.tabs";
const OPEN_KEY = "vesper.knowledge.open";
const VIEW_KEY = "vesper.knowledge.view";

/**
 * A value kept in localStorage and read with useSyncExternalStore: the server
 * render (and the first client render) see the fallback, then the stored
 * value, with no effect copying it into state.
 */
function localStore<T>(key: string, fallback: T) {
  let cache: T | undefined;
  const listeners = new Set<() => void>();
  const get = (): T => {
    if (cache === undefined) {
      try {
        cache = (JSON.parse(localStorage.getItem(key) ?? "null") as T) ?? fallback;
      } catch {
        cache = fallback;
      }
    }
    return cache;
  };
  return {
    get,
    server: () => fallback,
    set(next: T) {
      cache = next;
      localStorage.setItem(key, JSON.stringify(next));
      listeners.forEach((l) => l());
    },
    subscribe(l: () => void) {
      listeners.add(l);
      return () => listeners.delete(l);
    },
  };
}
function useStore<T>(s: ReturnType<typeof localStore<T>>): T {
  return useSyncExternalStore(s.subscribe, s.get, s.server);
}

const NO_TABS: Tab[] = [];
const DEFAULT_OPEN = ["Campaign", "Daily"];
const tabsStore = localStore<Tab[]>(TABS_KEY, NO_TABS);
const viewStore = localStore<View>(VIEW_KEY, "yours");
const openStore = localStore<string[]>(OPEN_KEY, DEFAULT_OPEN);

/** The tab a Knowledge address shows. */
function activeTab(pathname: string): string | undefined {
  if (pathname === "/knowledge") return "home";
  if (pathname.startsWith("/knowledge/graph")) return "graph";
  if (pathname.startsWith("/knowledge/connect")) return "connect";
  return /^\/knowledge\/([0-9a-f-]{36})/.exec(pathname)?.[1];
}
// The tab shown before the one being opened, so a new tab lands beside it.
let lastActive: string | undefined;

export function tabHref(t: Tab): string {
  if (t.kind === "graph") return "/knowledge/graph";
  if (t.kind === "connect") return "/knowledge/connect";
  if (t.kind === "home") return "/knowledge";
  return pageHref(t.id, t.title);
}

/**
 * Knowledge as a workspace, in the way Obsidian lays one out: the file
 * explorer on the left, tabs over the screen, and each screen's own right
 * sidebar and status bar. The Vesper sidebar folds to its icon rail while
 * it's open, as it does for the other apps.
 */
export function KnowledgeShell({ children }: { children: ReactNode }) {
  useEffect(() => claimEditorRail(), []);
  const router = useRouter();
  const pathname = usePathname();
  const view = useStore(viewStore);
  const tabs = useStore(tabsStore);
  const active = activeTab(pathname);
  const [switcher, setSwitcher] = useState(false);
  const [capture, setCapture] = useState(false);
  const [importing, setImporting] = useState(false);
  const daily = useDaily();

  const setView = useCallback((v: View) => viewStore.set(v), []);

  const openToday = useCallback(() => {
    daily.mutate({ date: isoDay(new Date()) }, { onSuccess: (p) => router.push(pageHref(p.id, p.title)) });
  }, [daily, router]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const mod = e.metaKey || e.ctrlKey;
      if (mod && !e.altKey && e.key.toLowerCase() === "o") {
        e.preventDefault();
        setSwitcher(true);
      } else if (mod && e.altKey && (e.key.toLowerCase() === "n" || e.code === "KeyN")) {
        e.preventDefault();
        setCapture(true);
      } else if (mod && e.key.toLowerCase() === "d" && !isTyping(e.target)) {
        e.preventDefault();
        openToday();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [openToday]);

  const registerTab = useCallback((tab: Tab) => {
    const cur = tabsStore.get();
    const next = openTab(cur, tab, lastActive);
    if (next !== cur) tabsStore.set(next);
    lastActive = tab.id;
  }, []);

  const shell: Shell = {
    view,
    setView,
    openSwitcher: () => setSwitcher(true),
    openCapture: () => setCapture(true),
    openImport: () => setImporting(true),
    openToday,
    registerTab,
  };

  const close = (id: string) => {
    const { tabs: left, next } = closeTab(tabs, id, active);
    tabsStore.set(left);
    if (id === active) router.push(next ? tabHref(next) : "/knowledge");
  };

  return (
    <Ctx.Provider value={shell}>
      <div className="flex h-[calc(100dvh-var(--tab-bar-h,0px))] min-h-0 min-w-0 flex-1 bg-bg md:h-dvh">
        <Explorer pathname={pathname} />
        <div className="flex min-w-0 flex-1 flex-col">
          <TabStrip tabs={tabs} active={active} onClose={close} />
          <div className="flex min-h-0 flex-1 flex-col">{children}</div>
        </div>
      </div>
      <QuickSwitcher open={switcher} onOpenChange={setSwitcher} includeAgent={view === "agent"} />
      <CaptureDialog open={capture} onOpenChange={setCapture} />
      <ImportDialog open={importing} onOpenChange={setImporting} />
    </Ctx.Provider>
  );
}

function TabStrip({ tabs, active, onClose }: { tabs: Tab[]; active?: string; onClose: (id: string) => void }) {
  const icon = (t: Tab): IconName => (t.kind === "graph" ? "graph" : t.kind === "connect" ? "link" : t.kind === "home" ? "dashboard" : "artifacts");
  return (
    <div role="tablist" aria-label="Open pages" className="flex h-10 flex-none items-end gap-1 overflow-x-auto border-b border-border-subtle bg-panel px-2">
      {tabs.map((t) => {
        const on = t.id === active;
        return (
          <div
            key={t.id}
            role="tab"
            aria-selected={on}
            className={cx(
              "group relative -mb-px flex h-[34px] max-w-[220px] flex-none items-center gap-2 rounded-t-lg pl-3 pr-1.5 text-xs font-medium",
              on ? "border-x border-t border-border-subtle bg-bg text-fg" : "text-fg-tertiary hover:bg-hover hover:text-fg-secondary",
            )}
          >
            <Icon name={icon(t)} size={13} className={on ? "text-[#B9AEFF]" : undefined} />
            <Link href={tabHref(t)} className="min-w-0 flex-1 truncate after:absolute after:inset-0" title={t.title}>
              {t.title || "Untitled"}
            </Link>
            <button
              type="button"
              aria-label={`Close ${t.title}`}
              onClick={() => onClose(t.id)}
              className="relative z-10 grid size-5 place-items-center rounded text-fg-tertiary opacity-60 hover:bg-hover hover:opacity-100 group-hover:opacity-100"
            >
              <Icon name="x" size={11} />
            </button>
          </div>
        );
      })}
      <Link href="/knowledge" aria-label="New tab" className="mb-2 grid size-6 flex-none place-items-center rounded text-fg-tertiary hover:bg-hover">
        <Icon name="plus" size={13} />
      </Link>
    </div>
  );
}

const pageIdOf = (pathname: string) => /^\/knowledge\/([0-9a-f-]{36})/.exec(pathname)?.[1];

function Explorer({ pathname }: { pathname: string }) {
  const k = useKnowledge();
  const router = useRouter();
  const agent = k.view === "agent";
  const pages = useAllPages(agent);
  const activeId = pageIdOf(pathname);
  const current = usePage(activeId).data;
  const create = useCreatePage();
  const tree = useMemo(() => buildTree((pages.data ?? []) as TreeRow[]), [pages.data]);
  const stored = useStore(openStore);
  // The folders down to the page you're on are open too, until you close one.
  const [closed, setClosed] = useState<Set<string>>(new Set());
  const open = useMemo(() => new Set([...stored, ...ancestors(current?.path).filter((f) => !closed.has(f))]), [stored, current?.path, closed]);
  const toggle = (path: string) => {
    if (open.has(path)) {
      openStore.set(stored.filter((f) => f !== path));
      setClosed((c) => new Set([...c, path]));
    } else {
      openStore.set([...stored, path]);
      setClosed((c) => new Set([...c].filter((f) => f !== path)));
    }
  };
  const newPage = () => {
    const folder = current?.path?.split("/").slice(0, -1).join("/");
    create.mutate({ title: "Untitled", path: folder ? `${folder}/Untitled ${Date.now().toString(36)}` : undefined, body_md: "" }, { onSuccess: (p) => router.push(pageHref(p.id, p.title)) });
  };


  return (
    <aside aria-label="Knowledge explorer" className="hidden w-64 flex-none flex-col border-r border-border-subtle bg-sidebar md:flex">
      <div className="flex h-10 flex-none items-center gap-1 px-3">
        <span className="flex-1 text-sm font-semibold text-fg">Knowledge</span>
        <ExplorerButton icon="edit" label="New page" onClick={newPage} />
        <ExplorerButton icon="graph" label="Graph view" href="/knowledge/graph" />
        <ExplorerButton icon="link" label="Keep notes connected" href="/knowledge/connect" />
      </div>
      <div role="radiogroup" aria-label="Which knowledge" className="mx-3 flex h-7 flex-none rounded-md border border-border-subtle bg-surface-sunken p-0.5">
        {(
          [
            ["yours", "Yours", undefined],
            ["agent", "Agent knowledge", "agents"],
          ] as const
        ).map(([v, label, icon]) => (
          <button
            key={v}
            role="radio"
            aria-checked={k.view === v}
            onClick={() => k.setView(v)}
            className={cx("flex flex-1 items-center justify-center gap-1.5 rounded text-xs font-medium", k.view === v ? "bg-selected text-fg" : "text-fg-tertiary hover:text-fg-secondary")}
          >
            {icon && <Icon name={icon} size={12} />}
            {label}
          </button>
        ))}
      </div>
      <button
        onClick={k.openSwitcher}
        className="mx-3 mt-2.5 flex h-[30px] flex-none items-center gap-2 rounded-md border border-border bg-surface px-2.5 text-left text-xs text-fg-tertiary hover:border-border-strong"
      >
        <Icon name="search" size={13} />
        <span className="flex-1">Search pages, #tags, props</span>
        <Kbd>⌘O</Kbd>
      </button>
      <nav aria-label="Pages" className="mt-3 min-h-0 flex-1 overflow-y-auto px-2 pb-3">
        {pages.isPending ? (
          <div className="space-y-2 px-2">
            {Array.from({ length: 8 }, (_, i) => (
              <Skeleton key={i} className="h-4" style={{ width: `${50 + ((i * 17) % 40)}%` }} />
            ))}
          </div>
        ) : tree.count === 0 ? (
          <p className="px-2 py-3 text-xs leading-relaxed text-fg-tertiary">{agent ? "No agent knowledge yet. Runs write their memory and proposed patterns here." : "No pages yet. Create one, capture a thought, or import your Obsidian vault."}</p>
        ) : (
          <FolderItems folder={tree} depth={0} open={open} toggle={toggle} activeId={activeId} />
        )}
      </nav>
      <div className="flex-none border-t border-border-subtle px-2 py-2">
        <button onClick={k.openToday} className="flex h-8 w-full items-center gap-2.5 rounded-md px-2 text-[13px] font-medium text-fg-2 hover:bg-hover">
          <Icon name="clock" size={14} className="text-fg-secondary" />
          Today
          <span className="text-xs font-normal text-fg-tertiary">{new Date().toLocaleDateString("en", { weekday: "short", month: "short", day: "numeric" })}</span>
        </button>
        <button onClick={k.openCapture} className="flex h-8 w-full items-center gap-2.5 rounded-md px-2 text-[13px] font-medium text-fg-2 hover:bg-hover">
          <Icon name="inbox" size={14} className="text-fg-secondary" />
          Quick capture
          <Kbd className="ml-auto">⌥⌘N</Kbd>
        </button>
        <button onClick={k.openImport} className="flex h-8 w-full items-center gap-2.5 rounded-md px-2 text-[13px] font-medium text-fg-2 hover:bg-hover">
          <Icon name="upload" size={14} className="text-fg-secondary" />
          Import Obsidian vault
        </button>
      </div>
    </aside>
  );
}

function ExplorerButton({ icon, label, onClick, href }: { icon: IconName; label: string; onClick?: () => void; href?: string }) {
  const cls = "grid size-7 place-items-center rounded-md text-fg-tertiary hover:bg-hover hover:text-fg-secondary";
  return href ? (
    <Link href={href} aria-label={label} title={label} className={cls}>
      <Icon name={icon} size={14} />
    </Link>
  ) : (
    <button type="button" aria-label={label} title={label} onClick={onClick} className={cls}>
      <Icon name={icon} size={14} />
    </button>
  );
}

function FolderItems({ folder, depth, open, toggle, activeId }: { folder: TreeFolder; depth: number; open: Set<string>; toggle: (p: string) => void; activeId?: string }) {
  return (
    <ul role={depth ? "group" : "tree"} className="space-y-px">
      {/* Inbox heads the tree, as the place new captures land. */}
      {[...folder.folders].sort((a, b) => (depth === 0 ? Number(b.name === "Inbox") - Number(a.name === "Inbox") : 0)).map((f) => {
        const isOpen = open.has(f.path);
        return (
          <li key={f.path} role="treeitem" aria-expanded={isOpen} aria-selected={false}>
            <button
              onClick={() => toggle(f.path)}
              style={{ paddingLeft: 6 + depth * 16 }}
              className="flex h-[26px] w-full items-center gap-1.5 rounded-md pr-2 text-[13px] font-medium text-fg-2 hover:bg-hover"
            >
              <Icon name={isOpen ? "chev" : "chevr"} size={12} className="text-fg-tertiary" />
              <Icon name={depth === 0 && f.name === "Inbox" ? "inbox" : "folder"} size={14} className="text-fg-secondary" />
              <span className="min-w-0 flex-1 truncate text-left">{f.name}</span>
              {((depth === 0 && f.name === "Inbox") || f.name === "Proposed") && <span className="rounded-full bg-accent-soft px-1.5 text-[10px] font-semibold text-[#B9AEFF]">{f.count}</span>}
            </button>
            {isOpen && <FolderItems folder={f} depth={depth + 1} open={open} toggle={toggle} activeId={activeId} />}
          </li>
        );
      })}
      {folder.pages.map((p) => (
        <li key={p.id} role="treeitem" aria-selected={p.id === activeId}>
          <TreeLink
            href={pageHref(p.id, p.title)}
            icon={p.properties?.query ? "board" : "artifacts"}
            label={p.title}
            depth={depth}
            active={p.id === activeId}
            dot={(p.pending ?? 0) > 0}
          />
        </li>
      ))}
    </ul>
  );
}

function TreeLink({ href, icon, label, depth, active, badge, dot }: { href: string; icon: IconName; label: string; depth: number; active?: boolean; badge?: number; dot?: boolean }) {
  return (
    <Link
      href={href}
      style={{ paddingLeft: 24 + depth * 16 }}
      aria-current={active ? "page" : undefined}
      className={cx(
        "relative flex h-[26px] items-center gap-1.5 rounded-md pr-2 text-[13px]",
        active ? "bg-selected font-medium text-fg before:absolute before:left-0 before:top-1.5 before:h-3.5 before:w-0.5 before:rounded before:bg-accent" : "text-fg-secondary hover:bg-hover",
      )}
    >
      <Icon name={icon} size={13} className={active ? "text-[#B9AEFF]" : "text-fg-tertiary"} />
      <span className="min-w-0 flex-1 truncate">{label}</span>
      {dot && <span title="Persona changes to review" className="size-1.5 rounded-full bg-state-human" />}
      {badge !== undefined && <span className="rounded-full bg-accent-soft px-1.5 text-[10px] font-semibold text-[#B9AEFF]">{badge}</span>}
    </Link>
  );
}
