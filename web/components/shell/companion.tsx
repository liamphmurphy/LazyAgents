"use client";

import { useCallback, useEffect, useId, useState, useSyncExternalStore, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { WorkChat } from "@/components/domain/work-chat";
import { claimCompanion, companionClaimed, companionSlot, setCompanionSlot, subscribeCompanion } from "@/lib/companion";
import { actOnDock, dockShortcut, loadDock, saveDock, type DockAction, type DockState } from "@/lib/editor-layout";
import { useCompact } from "@/lib/viewport";
import { Dock, type DockTab } from "./editor-layout";
import { isTyping } from "./shortcuts";

/**
 * Puts a page's own dock in the shell's right-hand column, in place of the default companion, so
 * every page's panel sits in the same place at the same width (DESIGN.md §3). The content keeps
 * this page's React context. Phones have no column: there the dock renders where it is.
 */
export function CompanionPortal({ children }: { children: ReactNode }) {
  const slot = useSyncExternalStore(subscribeCompanion, companionSlot, () => null);
  const compact = useCompact();
  useEffect(() => claimCompanion(), []);
  if (slot) return createPortal(children, slot);
  return compact ? <>{children}</> : null;
}

/**
 * A dock's state, remembered per `app` (its tab) and shared across pages (open or folded), with
 * ⌘\ to fold or reopen it. For docks that aren't an EditorLayout's: a run's, a ticket's, an
 * epic's and the default companion.
 */
export function useDockState(app: string, tabIds: string[], defaultOpen: boolean): [DockState, (a: DockAction) => void] {
  const key = tabIds.join("\n");
  const [state, setState] = useState<DockState>({ open: defaultOpen, tab: tabIds[0] ?? "", hidden: false });
  const [loaded, setLoaded] = useState(false);
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- reading browser-only storage after hydration
    setState(loadDock(window.localStorage, app, key.split("\n"), defaultOpen));
    setLoaded(true);
  }, [app, key, defaultOpen]);
  useEffect(() => {
    if (loaded) saveDock(window.localStorage, app, state);
  }, [app, state, loaded]);
  const dispatch = useCallback((a: DockAction) => setState((s) => actOnDock(s, a)), []);
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (dockShortcut(e, isTyping(e.target)) !== "toggle") return;
      e.preventDefault();
      dispatch({ type: "toggle" });
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [dispatch]);
  return [state, dispatch];
}

/** A page's own dock in the companion column: its tabs (its facts, then Chat), folding to a rail. */
export function CompanionDock({ app, tabs, defaultOpen = true }: { app: string; tabs: DockTab[]; defaultOpen?: boolean }) {
  const uid = useId();
  const [state, dispatch] = useDockState(app, tabs.map((t) => t.id), defaultOpen);
  return (
    <CompanionPortal>
      <Dock uid={uid} tabs={tabs} state={state} dispatch={dispatch} />
    </CompanionPortal>
  );
}

/** The work chat, as the companion on every page with no panel of its own. Starts folded to its rail. */
function DefaultCompanion() {
  const uid = useId();
  const tabs: DockTab[] = [{ id: "chat", label: "Chat", icon: "msg", content: <WorkChat /> }];
  const [state, dispatch] = useDockState("companion", ["chat"], false);
  return <Dock uid={uid} tabs={tabs} state={state} dispatch={dispatch} panels={state.open ? undefined : null} />;
}

/**
 * The shell's right-hand column (wide screens): the page's own dock when it has one, else the
 * default companion. Sticky to the viewport like the left column, and the positioning context a
 * narrow window's overlaid dock floats from.
 */
export function CompanionColumn() {
  const claimed = useSyncExternalStore(subscribeCompanion, companionClaimed, () => false);
  return (
    <div ref={setCompanionSlot} data-companion className="relative flex min-h-0 flex-none min-[612px]:sticky min-[612px]:top-0 min-[612px]:h-dvh">
      {!claimed && <DefaultCompanion />}
    </div>
  );
}

/** For a page whose own right-hand panel stays in its layout (the chat page's): no default companion beside it. */
export function useClaimCompanion() {
  useEffect(() => claimCompanion(), []);
}
