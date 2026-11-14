"use client";

import Link from "next/link";
import { useContext } from "react";
import { useChatCanvases } from "@/lib/api/canvas-hooks";
import { parseCanvasFence } from "@/lib/chat-canvas";
import { useCanvasPanel } from "@/lib/use-canvas-panel";
import { Icon } from "@/components/ui/icon";
import { CanvasView } from "./canvas-view";
import { CodeFence } from "./code-fence";
import { DesignCard } from "./design-card";
import { RecommendActions } from "./recommend-fence";
import { SvgFenceMessage } from "./svg-fence";
import { LoadingText } from "@/components/ui/loading";

/**
 * A ```canvas fence. The last card for a canvas is its live copy, which expands
 * in place; earlier cards stay collapsed and lead to it.
 */
export function CanvasFence({ code, info }: { code: string; info?: string }) {
  const id = parseCanvasFence(code);
  const actions = useContext(RecommendActions);
  const messageId = useContext(SvgFenceMessage);
  const chatId = actions?.chatId;
  const chat = actions?.canvas;
  const list = useChatCanvases(id ? chatId : undefined);
  const panel = useCanvasPanel();
  const canvas = list.data?.canvases.find((c) => c.id === id);
  if (!id || !chatId || (list.data && !canvas) || list.isError) return <CodeFence code={code} info={info} />;
  // A design opens beside the chat (design-panel), not in place.
  if (canvas?.kind === "design") return <DesignCard id={id!} title={canvas.title} />;
  // A deck has nothing to show in place (it is edited, presented and exported in Slides), so it is
  // always the one-line card, a link to the deck: never an expanded panel that only says so, and
  // in the deck's own Slides dock, never a second copy of the document beside it.
  if (canvas?.kind === "deck") return <CardLink href={`/apps/slides/${id}`} icon="slides" title={canvas.title} meta={`Slide deck · v${canvas.seq}`} />;
  const live = !!messageId && chat?.lastMessage(id) === messageId;
  const expanded = (panel.focus ?? (panel.collapsed ? undefined : chat?.newest)) === id;
  if (canvas && live && expanded && !panel.fullscreen) {
    return (
      <div data-canvas-card={id} data-testid="canvas-panel" className="flex h-[520px] w-[640px] max-w-full flex-col overflow-hidden rounded-xl border border-border-strong bg-surface-sunken">
        <CanvasView chatId={chatId} canvasId={id} live={!!chat?.live} readOnly={!!chat?.readOnly} agent={chat?.agent ?? "Assistant"} onCollapse={panel.collapse} onFullscreen={() => panel.openFull(id)} />
      </div>
    );
  }
  const open = () => {
    panel.show(id);
    if (!live) requestAnimationFrame(() => document.querySelector(`[data-canvas-card="${id}"]`)?.scrollIntoView({ block: "nearest", behavior: "smooth" }));
  };
  return (
    <button
      type="button"
      data-testid="canvas-card"
      data-canvas-card={live ? id : undefined}
      disabled={!canvas}
      onClick={open}
      className="flex w-[360px] max-w-full items-center gap-3 rounded-xl border border-border bg-surface px-3 py-2.5 text-left transition-colors hover:border-border-strong hover:bg-hover"
    >
      <Icon name={canvas?.kind === "svg" ? "library" : "edit"} size={16} className="flex-none text-fg-tertiary" />
      <span className="flex min-w-0 flex-col">
        <span className="truncate text-14 font-semibold text-fg">{canvas?.title ?? "Canvas"}</span>
        <span className="text-12 text-fg-secondary">{canvas ? `${canvas.kind === "svg" ? "SVG mockup" : "Document"} · v${canvas.seq}` : <LoadingText />}</span>
      </span>
    </button>
  );
}

function CardLink({ href, icon, title, meta }: { href: string; icon: "slides"; title: string; meta: string }) {
  return (
    <Link href={href} data-testid="canvas-card" className="flex w-[360px] max-w-full items-center gap-3 rounded-xl border border-border bg-surface px-3 py-2.5 text-left transition-colors hover:border-border-strong hover:bg-hover">
      <Icon name={icon} size={16} className="flex-none text-fg-tertiary" />
      <span className="flex min-w-0 flex-col">
        <span className="truncate text-14 font-semibold text-fg">{title}</span>
        <span className="text-12 text-fg-secondary">{meta}</span>
      </span>
    </Link>
  );
}
