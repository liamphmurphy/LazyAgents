// Which of a chat's side panels are open, in the URL (components: app/(app)/chat/_components/chat-panel-host).
// The params are the ones use-canvas-panel and use-playground-panel already read and write, so
// links and reloads behave as before; `panel` only picks which open panel a sheet shows.
// Kept free of React so `npm run test:unit` covers it.

export type ChatPanel = "design" | "canvas" | "playground";

/** How the host lays its panels out: beside the chat (desktop) or as one full-height sheet with a switch (compact). */
export type PanelLayout = "side" | "sheet";

export const PANELS: ChatPanel[] = ["design", "canvas", "playground"];

/** A tab in the desktop dock: the open panels, and Activity, which is always there. */
export type DockTabId = ChatPanel | "activity";

type Query = Pick<URLSearchParams, "get">;

/** The panels open now, in switch order. `designIds` are the chat's designs: a canvas focus on one of them opens the design panel. */
export function openPanels(q: Query, designIds: readonly string[]): ChatPanel[] {
  const focus = q.get("canvas");
  // "full" shows the canvas as a panel (the dock, or the compact sheet); "max" takes it over the window.
  const full = q.get("canvas_view") === "full" || q.get("canvas_view") === "max";
  const pg = q.get("playground");
  const open: ChatPanel[] = [];
  if (!full && focus && focus !== "none" && designIds.includes(focus)) open.push("design");
  if (full) open.push("canvas");
  if (pg === "open" || pg === "full") open.push("playground");
  return open;
}

/** The one panel a sheet shows: the one switched to, if it's still open, else the frontmost (a fullscreen canvas, then the playground, then a design). */
export function activePanel(q: Query, open: readonly ChatPanel[]): ChatPanel | undefined {
  const picked = q.get("panel") as ChatPanel | null;
  if (picked && open.includes(picked)) return picked;
  return (["canvas", "playground", "design"] as const).find((p) => open.includes(p));
}

/** The query that shows `panel`, opening it if it's closed. A design needs the id to open; without one the query is unchanged. */
export function switchPanel(q: URLSearchParams, panel: ChatPanel, opts: { designId?: string } = {}): URLSearchParams {
  const next = new URLSearchParams(q.toString());
  if (panel === "design") {
    if (!opts.designId) return next;
    next.set("canvas", opts.designId);
    next.delete("canvas_view");
  } else if (panel === "canvas") {
    next.set("canvas_view", "full");
  } else if (next.get("playground") !== "open" && next.get("playground") !== "full") {
    next.set("playground", "open");
  }
  next.set("panel", panel);
  next.delete("sheet");
  return next;
}

/** Whether the compact sheet is dismissed: back to the chat with its panels still open, until a switch shows it again. */
export function sheetHidden(q: Query): boolean {
  return q.get("sheet") === "hidden";
}

/** The query with the compact sheet dismissed; switchPanel shows it again. */
export function hideSheet(q: URLSearchParams): URLSearchParams {
  const next = new URLSearchParams(q.toString());
  next.set("sheet", "hidden");
  return next;
}

/** The query with `panel` closed; the others stay as they were. */
export function closePanel(q: URLSearchParams, panel: ChatPanel): URLSearchParams {
  const next = new URLSearchParams(q.toString());
  if (panel === "design") next.set("canvas", "none");
  else if (panel === "canvas") next.delete("canvas_view");
  else {
    next.delete("playground");
    next.delete("playground_tab");
  }
  if (next.get("panel") === panel) next.delete("panel");
  return next;
}

/** The tab the desktop dock shows: the one picked (Activity, or a panel still open), else the frontmost open panel, else Activity. */
export function dockTab(q: Query, open: readonly ChatPanel[]): DockTabId {
  if (q.get("panel") === "activity") return "activity";
  return activePanel(q, open) ?? "activity";
}

/**
 * Whether the desktop dock is open or collapsed to its rail: `dock=1` / `dock=0` once the person
 * chose, else open while a panel (or Activity) is picked or open, so a chat with nothing open starts on the rail.
 */
export function dockOpen(q: Query, open: readonly ChatPanel[]): boolean {
  const d = q.get("dock");
  if (d === "0") return false;
  if (d === "1") return true;
  return open.length > 0 || q.get("panel") === "activity";
}

/** The query showing the dock on `tab` (which must already be open), open or collapsed. */
export function setDock(q: URLSearchParams, s: { tab: DockTabId; open: boolean }): URLSearchParams {
  const next = new URLSearchParams(q.toString());
  next.set("panel", s.tab);
  next.set("dock", s.open ? "1" : "0");
  return next;
}

/**
 * Whether a drag down on the sheet's grabber, released `dy` px below where it started after `ms`,
 * closes the sheet: past a quarter of its height (at least 120px), or a quick flick of 40px or more.
 */
export function dragCloses(dy: number, ms: number, height: number): boolean {
  if (dy >= Math.max(120, height / 4)) return true;
  return dy >= 40 && dy / Math.max(ms, 1) > 0.5;
}

/** The threads a Comments button counts: top-level comments that aren't resolved (replies don't count). */
export function openThreads(comments: readonly { status: string; parent_id?: string | null }[] | undefined): number {
  return (comments ?? []).filter((c) => !c.parent_id && c.status !== "resolved").length;
}
