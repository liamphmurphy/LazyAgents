"use client";

import Link from "next/link";
import { useCallback, useEffect, useId, useRef, useState, useSyncExternalStore, type ReactNode } from "react";
import { Icon, type IconName } from "@/components/ui/icon";
import { Sheet } from "@/components/ui/sheet";
import { cx } from "@/lib/cx";
import {
  claimEditorRail,
  dockReducer,
  dockShortcut,
  editorRailActive,
  loadDock,
  saveDock,
  subscribeEditorRail,
  type DockAction,
  type DockState,
} from "@/lib/editor-layout";
import { useCompact } from "@/lib/viewport";
import { isTyping } from "./shortcuts";

/** A tab in the right dock. `busy` pulses its ring, `unread` adds a dot (Chat uses both for the persona). */
export type DockTab = {
  id: string;
  label: string;
  icon: IconName;
  /** Drawn instead of `icon`, e.g. the persona's avatar on Chat. */
  avatar?: ReactNode;
  badge?: ReactNode;
  busy?: boolean;
  unread?: boolean;
  content: ReactNode;
};

export type EditorHeader = {
  back: { href: string; label: string };
  title: ReactNode;
  /** The ▾ menu beside the title (rename, duplicate, …). */
  titleMenu?: ReactNode;
  saveState?: ReactNode;
  presence?: ReactNode;
  /** Up to two primary actions. */
  actions?: [ReactNode, ReactNode?];
  /** The ⋯ menu. */
  overflow?: ReactNode;
};

/** True while an EditorLayout is mounted: the Vesper sidebar then shows as its icon rail. */
export function useEditorRail(): boolean {
  return useSyncExternalStore(subscribeEditorRail, editorRailActive, () => false);
}

const COMPOSER = "[data-dock-composer], textarea, [contenteditable='true'], input:not([type='hidden'])";

/**
 * The layout every app editor uses (mockup "EditorLayout: Designs editor"): a header, an optional
 * structure panel on the left, the canvas, and a dock of tabs on the right that collapses to a 44px
 * rail in place. ⌘\ collapses or reopens the dock, ⌘J opens `chatTab` and focuses its composer, ⌘.
 * hides the left panel and the dock and restores them. The dock's open state and tab are remembered
 * per `app`. Below xl the open dock overlays the canvas; below md it opens as a sheet.
 */
export function EditorLayout({
  app,
  header,
  left,
  main,
  dock,
  chatTab = "chat",
  show,
  onDockChange,
}: {
  /** Key for the remembered dock state, e.g. "designs". */
  app: string;
  header: EditorHeader;
  left?: ReactNode;
  main: ReactNode;
  dock: DockTab[];
  chatTab?: string;
  /** Opens `tab` whenever `n` changes, e.g. when the canvas asks to start a review. */
  show?: { tab: string; n: number };
  /** The dock's state after every change, e.g. to clear an unread mark while its tab shows. */
  onDockChange?: (s: DockState) => void;
}) {
  const uid = useId();
  const compact = useCompact();
  const tabIds = dock.map((t) => t.id).join("\n");
  const [state, setState] = useState<DockState>({ open: true, tab: dock[0]?.id ?? "", hidden: false });
  const [loaded, setLoaded] = useState(false);
  // On phones the sheet starts closed whatever was remembered, and isn't remembered itself.
  const [sheetOpen, setSheetOpen] = useState(false);
  const [focusChat, setFocusChat] = useState(0);

  useEffect(() => claimEditorRail(), []);

  // Read after mount so the server render matches the first client render. Later tab changes (a
  // tab that comes and goes, like Review) keep the state and only move off a tab that left.
  const loadedApp = useRef<string>(undefined);
  useEffect(() => {
    const tabs = tabIds.split("\n");
    if (loadedApp.current !== app) {
      loadedApp.current = app;
      setState(loadDock(window.localStorage, app, tabs));
      setLoaded(true);
    } else setState((s) => (tabs.includes(s.tab) ? s : { ...s, tab: tabs[0] ?? "" }));
  }, [app, tabIds]);

  useEffect(() => {
    if (loaded) saveDock(window.localStorage, app, state);
  }, [app, state, loaded]);

  useEffect(() => {
    onDockChange?.(state);
  }, [state, onDockChange]);

  const dispatch = useCallback(
    (a: DockAction) => {
      setState((s) => dockReducer(s, a));
      if (!compact) return;
      if (a.type === "open" || a.type === "select") setSheetOpen(true);
      else if (a.type === "collapse" || a.type === "toggleHidden") setSheetOpen(false);
      else setSheetOpen((o) => !o);
    },
    [compact],
  );

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      const action = dockShortcut(e, isTyping(e.target));
      if (!action) return;
      e.preventDefault();
      if (action === "toggle") dispatch({ type: "toggle" });
      else if (action === "hideAll") dispatch({ type: "toggleHidden" });
      else if (dock.some((t) => t.id === chatTab)) {
        dispatch({ type: "open", tab: chatTab });
        setFocusChat((n) => n + 1);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [dispatch, dock, chatTab]);

  // A new `show` opens its tab (adjusting state while rendering, so it lands with the same paint).
  const showN = show?.n ?? 0;
  const [shownN, setShownN] = useState(showN);
  if (showN !== shownN) {
    setShownN(showN);
    if (show?.tab) {
      setState((s) => dockReducer(s, { type: "open", tab: show.tab }));
      if (compact) setSheetOpen(true);
    }
  }

  // After ⌘J renders the Chat panel (or the sheet's focus trap settles), focus its composer.
  useEffect(() => {
    if (!focusChat) return;
    const id = requestAnimationFrame(() => {
      const panel = document.querySelector(`[data-dock-panel="${CSS.escape(`${uid}:${chatTab}`)}"]:not([hidden])`);
      panel?.querySelector<HTMLElement>(COMPOSER)?.focus();
    });
    return () => cancelAnimationFrame(id);
  }, [focusChat, uid, chatTab]);

  const active = dock.find((t) => t.id === state.tab) ?? dock[0];
  const hasDock = dock.length > 0;
  const showLeft = !!left && !state.hidden;
  const docked = hasDock && !state.hidden;
  const dockOpen = docked && state.open && !compact;

  const panels = dock.map((t) => (
    <div
      key={t.id}
      role="tabpanel"
      id={`${uid}-panel-${t.id}`}
      aria-labelledby={compact ? undefined : `${uid}-tab-${t.id}`}
      aria-label={compact ? t.label : undefined}
      data-dock-panel={`${uid}:${t.id}`}
      hidden={t.id !== active?.id}
      className="flex min-h-0 flex-1 flex-col"
    >
      {t.content}
    </div>
  ));

  return (
    <div data-editor-layout={app} className="flex h-[calc(100dvh-var(--tab-bar-h,0px))] min-h-0 min-w-0 flex-1 flex-col bg-bg md:h-dvh">
      <Header {...header} />
      <div className="relative flex min-h-0 flex-1">
        {showLeft && (
          <aside aria-label="Structure" className="flex w-[240px] flex-none flex-col overflow-y-auto border-r border-border-subtle bg-sidebar max-md:hidden">
            {left}
          </aside>
        )}
        <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-auto bg-bg">{main}</div>
        {dockOpen && (
          <aside
            aria-label="Panel"
            className="flex w-[360px] flex-none flex-col border-l border-border-subtle bg-surface max-xl:absolute max-xl:inset-y-0 max-xl:right-0 max-xl:z-20 max-xl:shadow-[var(--shadow-float)]"
          >
            <div className="flex h-10 flex-none items-center gap-1 border-b border-border-subtle bg-sidebar px-2">
              <div role="tablist" aria-label="Panel tabs" className="flex min-w-0 flex-1 gap-1 overflow-x-auto">
                {dock.map((t) => (
                  <button
                    key={t.id}
                    type="button"
                    role="tab"
                    id={`${uid}-tab-${t.id}`}
                    aria-selected={t.id === active?.id}
                    aria-controls={`${uid}-panel-${t.id}`}
                    title={t.id === active?.id ? `${t.label}: click to collapse` : t.label}
                    onClick={() => dispatch({ type: "select", tab: t.id })}
                    className={cx(
                      "relative flex h-7 flex-none items-center gap-1.5 rounded-md px-2 text-[13px]",
                      t.id === active?.id ? "bg-selected font-medium text-fg" : "text-fg-secondary hover:bg-hover hover:text-fg",
                    )}
                  >
                    <TabIcon tab={t} />
                    {t.label}
                    {t.badge}
                  </button>
                ))}
              </div>
              <button
                type="button"
                aria-label="Collapse panel"
                title="Collapse panel (⌘\)"
                onClick={() => dispatch({ type: "collapse" })}
                className="grid size-7 flex-none place-items-center rounded-md border border-border text-fg-secondary hover:bg-hover hover:text-fg"
              >
                <span aria-hidden>⇥</span>
              </button>
            </div>
            {panels}
          </aside>
        )}
        {docked && !dockOpen && (
          <nav aria-label="Panel rail" className="flex w-[44px] flex-none flex-col items-center gap-2 border-l border-border-subtle bg-sidebar py-2.5">
            <button
              type="button"
              aria-label="Expand panel"
              title="Expand panel (⌘\)"
              onClick={() => dispatch({ type: "open", tab: active?.id ?? "" })}
              className="grid size-7 place-items-center rounded-md border border-border text-fg-secondary hover:bg-hover hover:text-fg"
            >
              <span aria-hidden>⇤</span>
            </button>
            {dock.map((t) => (
              <button
                key={t.id}
                type="button"
                aria-label={`Open ${t.label}`}
                title={t.label}
                onClick={() => dispatch({ type: "open", tab: t.id })}
                className="relative grid size-8 place-items-center rounded-full text-fg-secondary hover:bg-hover hover:text-fg"
              >
                <TabIcon tab={t} size={16} />
              </button>
            ))}
          </nav>
        )}
      </div>
      {compact && docked && (
        <Sheet open={sheetOpen} onOpenChange={(o) => (o ? setSheetOpen(true) : dispatch({ type: "collapse" }))} title={active?.label ?? ""} className="h-[85dvh]">
          <div className="flex h-full min-h-0 flex-col">{panels}</div>
        </Sheet>
      )}
    </div>
  );
}

function TabIcon({ tab, size = 14 }: { tab: DockTab; size?: number }) {
  return (
    <span className="relative grid place-items-center">
      {tab.busy && (
        <span aria-hidden data-busy className="absolute -inset-1.5 animate-pulse rounded-full border-2 border-accent motion-reduce:animate-none" />
      )}
      {tab.avatar ?? <Icon name={tab.icon} size={size} />}
      {tab.unread && <span aria-hidden data-unread className="absolute -right-1.5 -top-1.5 size-2 rounded-full bg-state-human" />}
      {(tab.busy || tab.unread) && <span className="sr-only">{[tab.busy && "working", tab.unread && "unread"].filter(Boolean).join(", ")}</span>}
    </span>
  );
}

function Header({ back, title, titleMenu, saveState, presence, actions, overflow }: EditorHeader) {
  return (
    <header className="flex h-12 flex-none items-center gap-3 border-b border-border-subtle bg-surface px-4">
      <Link href={back.href} className="flex flex-none items-center gap-1 text-[13px] text-fg-secondary hover:text-fg">
        <span aria-hidden>←</span>
        {back.label}
      </Link>
      <div className="flex min-w-0 items-center gap-1">
        <h1 className="truncate text-[14px] font-semibold">{title}</h1>
        {titleMenu}
      </div>
      {saveState && <span className="flex-none text-[12px] text-fg-tertiary" aria-live="polite">{saveState}</span>}
      <div className="flex-1" />
      {presence && <div className="flex flex-none items-center max-md:hidden">{presence}</div>}
      {actions && <div className="flex flex-none items-center gap-2">{actions}</div>}
      {overflow}
    </header>
  );
}
