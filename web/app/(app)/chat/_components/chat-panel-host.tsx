"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useChatCanvases } from "@/lib/api/canvas-hooks";
import { useChatPlayground } from "@/lib/api/playground-hooks";
import { activePanel, closePanel, openPanels, PANELS, switchPanel, type ChatPanel, type PanelLayout } from "@/lib/chat-panels";
import { cx } from "@/lib/cx";
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
    /** The panels there's something to show in, for a switch. */
    available: PANELS.filter((p) => (p === "design" ? designIds.length > 0 : p === "canvas" ? canvases.length > 0 : hasPlayground)),
    switchTo: (p: ChatPanel) => go(switchPanel(q(), p, { designId: designIds.at(-1) })),
    close: (p: ChatPanel) => go(closePanel(q(), p)),
  };
}

/**
 * A chat's panels: the design, canvas and playground. `side` (desktop) lays them out as they
 * always have been: the playground beside the chat, a design beside it, and the canvas and
 * playground fullscreen over the window. `sheet` (the compact shell) shows the open one as a
 * full-height sheet over the chat, with a Design / Canvas / Playground switch. Key it by chat.
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
  const active = panels.active;
  if (!active) return null;
  return (
    <section aria-label={LABELS[active]} data-testid="panel-sheet" data-panel={active} className="absolute inset-0 z-30 flex min-h-0 flex-col bg-panel">
      <div className="flex items-center gap-2 border-b border-border-subtle px-3 py-2">
        <span role="tablist" aria-label="Panel" className="flex flex-1 rounded-md bg-surface-sunken p-0.5">
          {panels.available.map((p) => (
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
      </div>
      {active === "design" && <DesignPanel chatId={chatId} live={live} sheet />}
      {active === "canvas" && <CanvasFullscreen chatId={chatId} live={live} readOnly={readOnly} agent={agent} inline />}
      {active === "playground" && <PlaygroundPanel chatId={chatId} sheet />}
    </section>
  );
}
