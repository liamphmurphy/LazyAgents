"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useEffect, useRef, useState, type PointerEvent as ReactPointerEvent } from "react";
import { useCanvasComments, useChatCanvases } from "@/lib/api/canvas-hooks";
import { useChatPlayground } from "@/lib/api/playground-hooks";
import { useDesignProposals } from "@/lib/api/designs-hooks";
import { activePanel, closePanel, dragCloses, hideSheet, openPanels, openThreads, PANELS, sheetHidden, switchPanel, type ChatPanel, type PanelLayout } from "@/lib/chat-panels";
import { cx } from "@/lib/cx";
import { useCanvasPanel } from "@/lib/use-canvas-panel";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { CanvasFullscreen } from "@/components/domain/canvas-view";
import { PlaygroundFullscreen, PlaygroundPanel } from "@/components/domain/playground";
import { DesignPanel, useOpenNewDesigns } from "./design-panel";

const LABELS: Record<ChatPanel, string> = { design: "Design", canvas: "Canvas", playground: "Playground" };

/** Which of a chat's panels are open and which a sheet shows, with the moves between them (lib/chat-panels). */
export function useChatPanels(chatId: string, live: boolean) {
  const router = useRouter();
  const pathname = usePathname();
  const search = useSearchParams();
  const canvases = useChatCanvases(chatId, live).data?.canvases ?? [];
  const designIds = canvases.filter((c) => c.kind === "design").map((c) => c.id);
  const hasPlayground = !!useChatPlayground(chatId).data;
  const open = openPanels(search, designIds);
  const go = (q: URLSearchParams) => router.replace(`${pathname}?${q.toString()}`, { scroll: false });
  const q = () => new URLSearchParams(search.toString());
  return {
    open,
    active: activePanel(search, open),
    /** The panels there's something to show in, for the compact header's Panels button. */
    available: PANELS.filter((p) => (p === "design" ? designIds.length > 0 : p === "canvas" ? canvases.length > 0 : hasPlayground)),
    /** Whether the compact sheet is dismissed (the panels stay open); switchTo shows it again. */
    hidden: sheetHidden(search),
    hide: () => go(hideSheet(q())),
    /** Shows `p`, already open, in the sheet (undismissed), without touching the rest of the query. */
    pick: (p: ChatPanel) => {
      const next = q();
      next.set("panel", p);
      next.delete("sheet");
      go(next);
    },
    switchTo: (p: ChatPanel) => go(switchPanel(q(), p, { designId: designIds.at(-1) })),
    close: (p: ChatPanel) => go(closePanel(q(), p)),
  };
}

/**
 * A chat's panels: the design, canvas and playground. `side` (desktop) lays them out as they
 * always have been: the playground beside the chat, a design beside it, and the canvas and
 * playground fullscreen over the window. `sheet` (the compact shell) shows the open one as a
 * full-height sheet over the chat ("4 Panel sheet"): a grabber, a switch over the open panels,
 * a close button, and a bottom bar with Comments and Review. Key it by chat.
 */
export function ChatPanelHost({ chatId, live, readOnly, agent, layout = "side" }: { chatId: string; live: boolean; readOnly: boolean; agent: string; layout?: PanelLayout }) {
  useOpenNewDesigns(chatId, live);
  const panels = useChatPanels(chatId, live);
  if (layout === "side") {
    return (
      <>
        {/* The playground sits beside the chat: a running app needs the width more than a canvas does. */}
        <DesignPanel chatId={chatId} live={live} />
        <PlaygroundPanel chatId={chatId} />
        <PlaygroundFullscreen chatId={chatId} />
        {panels.open.includes("canvas") ? <CanvasFullscreen chatId={chatId} live={live} readOnly={readOnly} agent={agent} /> : null}
      </>
    );
  }
  return <PanelSheet chatId={chatId} live={live} readOnly={readOnly} agent={agent} panels={panels} />;
}

function PanelSheet({ chatId, live, readOnly, agent, panels }: { chatId: string; live: boolean; readOnly: boolean; agent: string; panels: ReturnType<typeof useChatPanels> }) {
  const router = useRouter();
  const focus = useCanvasPanel().focus;
  const canvases = useChatCanvases(chatId, live).data?.canvases;
  const { open, active, hidden, hide } = panels;

  // A panel opening (a card's Open, a design the persona starts) comes to the front, bringing a dismissed sheet back.
  const key = open.join(",");
  const seen = useRef(open);
  useEffect(() => {
    const opened = open.filter((p) => !seen.current.includes(p));
    seen.current = open;
    if (!opened.length) return;
    panels.pick(opened.at(-1)!);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- runs when the set of open panels changes
  }, [key]);

  // Escape returns to the chat, unless the panel used it (a design dropping its selection) or focus is in a field.
  useEffect(() => {
    if (hidden || !active) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape" || e.defaultPrevented || (e.target as Element).closest?.("input, textarea, select, [contenteditable]")) return;
      hide();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [hidden, active, hide]);

  // Drag the grabber down to return to the chat. Followed on the window, so the drag carries on past the grabber.
  const ref = useRef<HTMLElement>(null);
  const [dy, setDy] = useState(0);
  const startDrag = (e: ReactPointerEvent) => {
    if (e.button > 0 || (e.target as Element).closest("button")) return;
    const id = e.pointerId;
    const y0 = e.clientY;
    const t0 = e.timeStamp;
    const move = (m: PointerEvent) => m.pointerId === id && setDy(Math.max(0, m.clientY - y0));
    const stop = (u: PointerEvent) => {
      if (u.pointerId !== id) return;
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", stop);
      window.removeEventListener("pointercancel", stop);
      setDy(0);
      if (u.type === "pointerup" && dragCloses(u.clientY - y0, u.timeStamp - t0, ref.current?.clientHeight ?? 0)) hide();
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", stop);
    window.addEventListener("pointercancel", stop);
  };

  const designId = active === "design" ? focus : undefined;
  const pending = useDesignProposals(designId ?? "", !!designId).data?.length ?? 0;
  const canvasId = active === "canvas" ? (focus ?? canvases?.at(-1)?.id) : undefined;
  const threads = openThreads(useCanvasComments(chatId, canvasId, live).data?.comments);
  if (!active) return null;
  const showComments = () => ref.current?.querySelector('[data-panel-body]:not([hidden]) aside[aria-label="Comments"]')?.scrollIntoView({ block: "start", behavior: "smooth" });
  const bar = active === "canvas" || pending > 0;

  return (
    <section
      ref={ref}
      aria-label="Panels"
      data-testid="panel-sheet"
      data-panel={active}
      hidden={hidden}
      style={dy > 0 ? { transform: `translateY(${dy}px)` } : undefined}
      className={cx("absolute inset-0 z-30 flex min-h-0 flex-col bg-panel shadow-[var(--shadow-float)]", !dy && "transition-transform", hidden && "hidden")}
    >
      <div
        data-testid="panel-sheet-grabber"
        className="flex-none touch-none select-none border-b border-border-subtle px-3 pb-2"
        onPointerDown={startDrag}
      >
        <div aria-hidden className="mx-auto mb-1.5 mt-2 h-1 w-9 rounded-full bg-border" />
        <div className="flex items-center gap-2">
          <span role="tablist" aria-label="Panel" className="flex flex-1 rounded-md bg-surface-sunken p-0.5">
            {open.map((p) => (
              <button
                key={p}
                type="button"
                role="tab"
                aria-selected={p === active}
                onClick={() => panels.switchTo(p)}
                className={cx("min-h-[40px] flex-1 rounded-sm px-2 text-[13px]", p === active ? "bg-surface font-semibold text-fg shadow-sm" : "text-fg-secondary")}
              >
                {LABELS[p]}
              </button>
            ))}
          </span>
          <button type="button" aria-label="Close panels" onClick={hide} className="grid size-[44px] flex-none place-items-center rounded-md text-fg-secondary hover:bg-hover hover:text-fg">
            <Icon name="x" size={16} />
          </button>
        </div>
      </div>
      {/* Every open panel stays mounted, so switching doesn't reload a playground or lose a canvas's scroll. */}
      {open.map((p) => (
        <div key={p} data-panel-body={p} hidden={p !== active} className={cx("flex min-h-0 flex-1 flex-col", p !== active && "hidden")}>
          {p === "design" && <DesignPanel chatId={chatId} live={live} sheet />}
          {p === "canvas" && <CanvasFullscreen chatId={chatId} live={live} readOnly={readOnly} agent={agent} inline />}
          {p === "playground" && <PlaygroundPanel chatId={chatId} sheet />}
        </div>
      ))}
      {bar && (
        <div data-testid="panel-sheet-bar" className="flex flex-none items-center gap-2 border-t border-border-subtle px-3 py-2">
          {active === "canvas" && (
            <Button variant="secondary" className="min-h-[44px]" disabled={!threads} onClick={showComments}>
              Comments{threads ? ` ${threads}` : ""}
            </Button>
          )}
          <span className="flex-1" />
          {pending > 0 && designId && (
            <Button variant="approve" className="min-h-[44px]" onClick={() => router.push(`/apps/designs/${designId}?review=1`)}>
              Review {pending} {pending === 1 ? "suggestion" : "suggestions"}
            </Button>
          )}
        </div>
      )}
    </section>
  );
}
