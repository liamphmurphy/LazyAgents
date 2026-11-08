"use client";

import { useContext } from "react";
import { useChatCanvases } from "@/lib/api/canvas-hooks";
import { parseCanvasFence } from "@/lib/chat-canvas";
import { useCanvasPanel } from "@/lib/use-canvas-panel";
import { Icon } from "@/components/ui/icon";
import { CodeFence } from "./code-fence";
import { RecommendActions } from "./recommend-fence";

/** A ```canvas fence: a card that opens the canvas beside the conversation. */
export function CanvasFence({ code, info }: { code: string; info?: string }) {
  const id = parseCanvasFence(code);
  const chatId = useContext(RecommendActions)?.chatId;
  const list = useChatCanvases(id ? chatId : undefined);
  const panel = useCanvasPanel();
  const canvas = list.data?.canvases.find((c) => c.id === id);
  if (!id || !chatId || (list.data && !canvas) || list.isError) return <CodeFence code={code} info={info} />;
  return (
    <button
      type="button"
      data-testid="canvas-card"
      disabled={!canvas}
      aria-pressed={panel.focus === id}
      onClick={() => panel.show(id)}
      className="flex w-[360px] max-w-full items-center gap-3 rounded-xl border border-border bg-surface px-3 py-2.5 text-left transition-colors hover:border-border-strong hover:bg-hover"
    >
      <Icon name={canvas?.kind === "svg" ? "library" : "edit"} size={16} className="flex-none text-fg-tertiary" />
      <span className="flex min-w-0 flex-col">
        <span className="truncate text-[13.5px] font-semibold text-fg">{canvas?.title ?? "Canvas"}</span>
        <span className="text-xs text-fg-secondary">{canvas ? `${canvas.kind === "svg" ? "SVG mockup" : "Document"} · v${canvas.seq}` : "Loading…"}</span>
      </span>
    </button>
  );
}
