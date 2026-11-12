// State, persistence and shortcuts for the shared editor layout
// (components/shell/editor-layout.tsx), kept free of React so node:test covers them.

/** The dock's state: open or collapsed to its rail, which tab shows, and whether ⌘. hid every panel. */
export type DockState = { open: boolean; tab: string; hidden: boolean };

export type DockAction =
  | { type: "toggle" } // ⌘\ and the ⇥ / ⇤ buttons
  | { type: "select"; tab: string } // a tab in the open dock: the active one collapses it
  | { type: "open"; tab: string } // a rail icon or ⌘J: always opens that tab
  | { type: "collapse" }
  | { type: "toggleHidden" }; // ⌘.

export function dockReducer(s: DockState, a: DockAction): DockState {
  switch (a.type) {
    case "toggle":
      return s.hidden ? { ...s, hidden: false, open: true } : { ...s, open: !s.open };
    case "select":
      if (!s.hidden && s.open && s.tab === a.tab) return { ...s, open: false };
      return { open: true, tab: a.tab, hidden: false };
    case "open":
      return { open: true, tab: a.tab, hidden: false };
    case "collapse":
      return { ...s, open: false };
    case "toggleHidden":
      return { ...s, hidden: !s.hidden };
  }
}

/** localStorage key for one app's dock, e.g. lazyagents.editor-dock.designs. */
export const dockKey = (app: string) => `lazyagents.editor-dock.${app}`;

type KV = Pick<Storage, "getItem" | "setItem">;

/**
 * The remembered dock for `app`: open unless stored otherwise, on the stored tab if it still
 * exists, else the first. Never hidden: ⌘. lasts for the visit only.
 */
export function loadDock(store: KV | undefined, app: string, tabs: string[]): DockState {
  const fallback: DockState = { open: true, tab: tabs[0] ?? "", hidden: false };
  try {
    const raw = store?.getItem(dockKey(app));
    if (!raw) return fallback;
    const v = JSON.parse(raw) as { open?: unknown; tab?: unknown };
    return {
      open: typeof v.open === "boolean" ? v.open : true,
      tab: typeof v.tab === "string" && tabs.includes(v.tab) ? v.tab : fallback.tab,
      hidden: false,
    };
  } catch {
    return fallback;
  }
}

export function saveDock(store: KV | undefined, app: string, s: DockState) {
  try {
    store?.setItem(dockKey(app), JSON.stringify({ open: s.open, tab: s.tab }));
  } catch {
    /* No storage: not remembered. */
  }
}

export type DockShortcut = "toggle" | "chat" | "hideAll";

/**
 * The layout's shortcut for a keydown: ⌘\ toggles the dock, ⌘J opens Chat, ⌘. hides or restores
 * every panel. Ctrl counts as ⌘ off the Mac. While typing only ⌘J applies.
 */
export function dockShortcut(e: { key: string; metaKey: boolean; ctrlKey: boolean; altKey: boolean; shiftKey: boolean }, typing: boolean): DockShortcut | null {
  if (!(e.metaKey || e.ctrlKey) || e.altKey || e.shiftKey) return null;
  const k = e.key.toLowerCase();
  if (k === "j") return "chat";
  if (typing) return null;
  if (k === "\\") return "toggle";
  if (k === ".") return "hideAll";
  return null;
}

// While any editor layout is mounted the Vesper sidebar shows as its icon rail. A count, not a flag,
// so a layout replacing another during navigation never drops the override in between.
let editors = 0;
const listeners = new Set<() => void>();
const notify = () => listeners.forEach((l) => l());

/** Mark an editor layout mounted; call the returned function on unmount. */
export function claimEditorRail(): () => void {
  editors++;
  notify();
  let released = false;
  return () => {
    if (released) return;
    released = true;
    editors--;
    notify();
  };
}

export const editorRailActive = () => editors > 0;

export function subscribeEditorRail(l: () => void): () => void {
  listeners.add(l);
  return () => listeners.delete(l);
}

// ⌘. in the chat ("reading focus") hides the Vesper sidebar too, for as long as it's claimed,
// without touching its remembered preference. A count, like the editor rail.
let focusClaims = 0;
const focusListeners = new Set<() => void>();
const notifyFocus = () => focusListeners.forEach((l) => l());

/** Hide the Vesper sidebar; call the returned function to show it again. */
export function claimReadingFocus(): () => void {
  focusClaims++;
  notifyFocus();
  let released = false;
  return () => {
    if (released) return;
    released = true;
    focusClaims--;
    notifyFocus();
  };
}

export const readingFocusActive = () => focusClaims > 0;

export function subscribeReadingFocus(l: () => void): () => void {
  focusListeners.add(l);
  return () => focusListeners.delete(l);
}
