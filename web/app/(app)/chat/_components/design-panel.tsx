"use client";

import { useEffect, useRef } from "react";
import { useChatCanvases } from "@/lib/api/canvas-hooks";
import { useCanvasPanel } from "@/lib/use-canvas-panel";
import { DesignEditor } from "@/app/(app)/apps/designs/_components/design-editor";

/**
 * A design open beside the chat (docs/design/designs-from-chat.svg): the editor with a
 * cut-down toolbar and Pause, chosen through use-canvas-panel like a canvas card. A design
 * the persona starts while the chat is open (design_create) opens here by itself.
 */
/** Opens a design the persona starts while the chat is open (design_create). Key the caller by chat. */
export function useOpenNewDesigns(chatId: string, live: boolean) {
  const panel = useCanvasPanel();
  const list = useChatCanvases(chatId, live).data?.canvases;
  const ids = (list ?? []).filter((c) => c.kind === "design").map((c) => c.id);
  const key = ids.join(",");
  // The designs there were when the chat opened; one that appears after is new and opens.
  const seen = useRef<Set<string> | null>(null);
  const show = panel.show;
  useEffect(() => {
    if (!list) return;
    if (!seen.current) {
      seen.current = new Set(ids);
      return;
    }
    const fresh = ids.filter((id) => !seen.current!.has(id));
    for (const id of fresh) seen.current.add(id);
    if (fresh.length) show(fresh[fresh.length - 1]);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- runs when the set of designs changes
  }, [key, !!list]);
  return ids;
}

/** `sheet` fills the panel sheet (chat-panel-host) instead of sitting beside the chat. */
export function DesignPanel({ chatId, live, sheet }: { chatId: string; live: boolean; sheet?: boolean }) {
  const panel = useCanvasPanel();
  const list = useChatCanvases(chatId, live).data?.canvases;
  const ids = (list ?? []).filter((c) => c.kind === "design").map((c) => c.id);
  const open = panel.focus && ids.includes(panel.focus) && !panel.fullscreen ? panel.focus : undefined;
  if (!open) return null;
  return (
    <aside
      aria-label="Design beside the chat"
      data-testid="design-panel"
      className={
        sheet
          ? "flex min-h-0 flex-1 flex-col bg-surface-sunken"
          : "flex min-h-0 w-[540px] max-w-[55%] flex-none flex-col border-l border-border-subtle bg-surface-sunken max-lg:absolute max-lg:inset-y-0 max-lg:right-0 max-lg:z-30 max-lg:w-full max-lg:max-w-none max-lg:shadow-[var(--shadow-float)]"
      }
    >
      <DesignEditor key={open} id={open} beside={{ onClose: panel.collapse }} />
    </aside>
  );
}
