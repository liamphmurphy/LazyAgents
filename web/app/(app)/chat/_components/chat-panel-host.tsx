"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useEffect, useRef, useState, type PointerEvent as ReactPointerEvent } from "react";
import { useCanvasComments, useChatCanvases } from "@/lib/api/canvas-hooks";
import { useDesignProposals } from "@/lib/api/designs-hooks";
import { activePanel, closePanel, dragCloses, openPanels, openThreads, switchPanel, type ChatPanel, type PanelLayout } from "@/lib/chat-panels";
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
  const open = openPanels(search, designIds);
  const go = (q: URLSearchParams) => router.replace(`${pathname}?${q.toString()}`, { scroll: false });
  const q = () => new URLSearchParams(search.toString());
  return {
    open,
    active: activePanel(search, open),
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
export function ChatPanelHost({ chatId, live, readOnly, agent, layout = "side", sheet }: { chatId: string; live: boolean; readOnly: boolean; agent: string; layout?: PanelLayout; sheet?: SheetState }) {
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
  return <PanelSheet chatId={chatId} live={live} readOnly={readOnly} agent={agent} panels={panels} sheet={sheet} />;
}

/** The compact header's way back to a dismissed sheet: shown while a panel is open. */
export function PanelsButton({ chatId, live, onShow }: { chatId: string; live: boolean; onShow: () => void }) {
  const { open } = useChatPanels(chatId, live);
  if (!open.length) return null;
  return (
    <button type="button" aria-label={`Panels, ${open.length} open`} onClick={onShow} className="inline-flex h-[30px] flex-none items-center gap-1.5 rounded-md border border-accent bg-accent-soft px-3 text-[13px] text-fg transition-colors hover:bg-hover">
      Panels
      <span className="text-fg-tertiary">{open.length}</span>
    </button>
  );
}

/** Whether the compact sheet is dismissed (back to the chat, panels still open); the header's Panels button shows it again. */
export type SheetState = { hidden: boolean; hide: () => void; show: () => void };

function PanelSheet({ chatId, live, readOnly, agent, panels, sheet }: { chatId: string; live: boolean; readOnly: boolean; agent: string; panels: ReturnType<typeof useChatPanels>; sheet?: SheetState }) {
  const router = useRouter();
  const focus = useCanvasPanel().focus;
  const canvases = useChatCanvases(chatId, live).data?.canvases;
  const { open, active } = panels;
  const hidden = !!sheet?.hidden;
  const hide = sheet?.hide;
  const show = sheet?.show;

  // A panel opening (a card's Open, a design the persona starts) brings a dismissed sheet back.
  const key = open.join(",");
  const seen = useRef(open);
  useEffect(() => {
    if (open.some((p) => !seen.current.includes(p))) show?.();
    seen.current = open;
    // eslint-disable-next-line react-hooks/exhaustive-deps -- runs when the set of open panels changes
  }, [key]);

  // Escape returns to the chat, unless the panel used it (a design dropping its selection) or focus is in a field.
  useEffect(() => {
    if (hidden || !active || !hide) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape" || e.defaultPrevented || (e.target as Element).closest?.("input, textarea, select, [contenteditable]")) return;
      hide();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [hidden, active, hide]);

  // Drag the grabber down to return to the chat.
  const ref = useRef<HTMLElement>(null);
  const drag = useRef<{ y: number; t: number; id: number } | null>(null);
  const [dy, setDy] = useState(0);
  const end = (e: ReactPointerEvent) => {
    const d = drag.current;
    drag.current = null;
    setDy(0);
    if (d && dragCloses(e.clientY - d.y, e.timeStamp - d.t, ref.current?.clientHeight ?? 0)) hide?.();
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
        onPointerDown={(e) => {
          if ((e.target as Element).closest("button")) return;
          drag.current = { y: e.clientY, t: e.timeStamp, id: e.pointerId };
          e.currentTarget.setPointerCapture(e.pointerId);
        }}
        onPointerMove={(e) => drag.current && setDy(Math.max(0, e.clientY - drag.current.y))}
        onPointerUp={end}
        onPointerCancel={() => ((drag.current = null), setDy(0))}
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
                className={cx("min-h-9 flex-1 rounded-sm px-2 text-[13px]", p === active ? "bg-surface font-semibold text-fg shadow-sm" : "text-fg-secondary")}
              >
                {LABELS[p]}
              </button>
            ))}
          </span>
          {hide && (
            <button type="button" aria-label="Close panels" onClick={hide} className="grid size-9 flex-none place-items-center rounded-md text-fg-secondary hover:bg-hover hover:text-fg">
              <Icon name="x" size={16} />
            </button>
          )}
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
            <Button variant="secondary" className="min-h-10" disabled={!threads} onClick={showComments}>
              Comments{threads ? ` ${threads}` : ""}
            </Button>
          )}
          <span className="flex-1" />
          {pending > 0 && designId && (
            <Button variant="approve" className="min-h-10" onClick={() => router.push(`/apps/designs/${designId}?review=1`)}>
              Review {pending} {pending === 1 ? "suggestion" : "suggestions"}
            </Button>
          )}
        </div>
      )}
    </section>
  );
}
