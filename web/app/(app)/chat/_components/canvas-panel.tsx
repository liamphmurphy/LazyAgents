"use client";

import { useMemo, useState, useId } from "react";
import { useChatCanvas, useChatCanvases } from "@/lib/api/canvas-hooks";
import { cx } from "@/lib/cx";
import { sanitizeSvg } from "@/lib/svg-sanitize";
import { parseSvg } from "@/lib/svg-edit";
import { useCanvasPanel } from "@/lib/use-canvas-panel";
import { CodeFence } from "@/components/domain/code-fence";
import { Markdown } from "@/components/domain/markdown";
import { renderNode, viewBoxOf } from "@/components/domain/mockup-canvas";
import { IconButton } from "@/components/ui/button";

const errorText = (e: unknown) => (e instanceof Error ? e.message : "Something went wrong. Please try again.");

/** Header entry point, once the chat has a canvas; toggles the pane. */
export function CanvasButton({ chatId, live }: { chatId: string; live: boolean }) {
  const panel = useCanvasPanel();
  const total = useChatCanvases(chatId, live).data?.canvases.length ?? 0;
  if (!total) return null;
  return (
    <button
      type="button"
      aria-pressed={panel.open}
      aria-label={`Canvas, ${total} open`}
      onClick={() => (panel.open ? panel.close() : panel.show())}
      className={cx(
        "inline-flex h-[30px] items-center gap-1.5 rounded-md border px-3 text-[13px] transition-colors",
        panel.open ? "border-accent bg-selected text-fg" : "border-btn-secondary-border bg-btn-secondary text-fg hover:bg-hover",
      )}
    >
      Canvas
      <span className="text-fg-tertiary">{total}</span>
    </button>
  );
}

/** Rendered from the sanitized tree, not an <img>, so elements can be targeted. */
function SvgView({ source, title }: { source: string; title: string }) {
  const prefix = `cv-${useId().replace(/\W/g, "")}-`;
  const san = useMemo(() => {
    const doc = parseSvg(source);
    return doc ? sanitizeSvg(doc.documentElement, prefix) : null;
  }, [source, prefix]);
  if (!san?.tree) return <p role="alert" className="text-xs text-pill-blocked-fg">This SVG couldn&apos;t be displayed. Its source is under Source.</p>;
  const vb = viewBoxOf(san.tree.attrs);
  const { width: _w, height: _h, x: _x, y: _y, ...rootAttrs } = san.tree.attrs;
  void [_w, _h, _x, _y];
  return (
    <div className="flex flex-col gap-2">
      {san.skipped > 0 && (
        <p className="text-xs text-fg-tertiary">
          {san.skipped} element{san.skipped === 1 ? "" : "s"} can&apos;t be shown here. They&apos;re kept in the source.
        </p>
      )}
      <div className="overflow-auto rounded-lg bg-white p-2">
        <svg {...rootAttrs} viewBox={`${vb.x} ${vb.y} ${vb.w} ${vb.h}`} role="img" aria-label={title} data-testid="canvas-svg" className="mx-auto block h-auto w-full" style={{ aspectRatio: `${vb.w} / ${vb.h}` }}>
          {san.tree.children.map(renderNode)}
        </svg>
      </div>
    </div>
  );
}

function CanvasView({ chatId, canvasId, live, onClose, picker }: { chatId: string; canvasId: string; live: boolean; onClose: () => void; picker: React.ReactNode }) {
  // Undefined follows the current version as the agent revises it.
  const [seq, setSeq] = useState<number>();
  const [view, setView] = useState<"preview" | "source">("preview");
  const q = useChatCanvas(chatId, canvasId, seq, live);
  const c = q.data;
  const viewing = c?.viewing_seq;
  const note = c?.versions.find((v) => v.seq === viewing)?.note;
  const step = (to: number) => setSeq(c && to >= c.seq ? undefined : to);
  const tab = (v: "preview" | "source", label: string) => (
    <button type="button" role="tab" aria-selected={view === v} className={cx("rounded px-1.5", view === v ? "bg-surface text-fg" : "hover:text-fg")} onClick={() => setView(v)}>
      {label}
    </button>
  );
  return (
    <>
      <div className="flex items-center gap-2 border-b border-border-subtle px-4 py-2.5">
        <div className="min-w-0 flex-1">{picker ?? <h2 className="truncate text-[14px] font-semibold text-fg">{c?.title ?? "Canvas"}</h2>}</div>
        <IconButton icon="x" label="Close canvas" onClick={onClose} />
      </div>
      <div className="flex items-center gap-2 border-b border-border-row px-4 py-1.5 text-xs text-fg-tertiary">
        {c && viewing !== undefined && (
          <span className="flex items-center gap-1">
            <button type="button" aria-label="Previous version" disabled={viewing <= 1} className="rounded px-1 hover:text-fg disabled:opacity-40" onClick={() => step(viewing - 1)}>‹</button>
            <span data-testid="canvas-version">v{viewing} of {c.seq}</span>
            <button type="button" aria-label="Next version" disabled={viewing >= c.seq} className="rounded px-1 hover:text-fg disabled:opacity-40" onClick={() => step(viewing + 1)}>›</button>
          </span>
        )}
        {note && <span className="min-w-0 flex-1 truncate">{note}</span>}
        <span role="tablist" aria-label="Canvas view" className="ml-auto flex items-center gap-0.5">
          {tab("preview", "Preview")}
          {tab("source", "Source")}
        </span>
      </div>
      <div className="min-h-0 flex-1 overflow-auto p-4">
        {q.isError ? (
          <p role="alert" className="text-xs text-pill-blocked-fg">{errorText(q.error)}</p>
        ) : !c ? (
          <p className="text-xs text-fg-tertiary">Loading…</p>
        ) : view === "source" ? (
          <CodeFence code={c.body} info={c.kind === "svg" ? "html" : "markdown"} />
        ) : c.kind === "svg" ? (
          <SvgView source={c.body} title={c.title} />
        ) : (
          <Markdown className="type-reading text-fg-2">{c.body}</Markdown>
        )}
      </div>
    </>
  );
}

/** The pane beside a chat: read a canvas and step through its versions. */
export function CanvasPanel({ chatId, live }: { chatId: string; live: boolean }) {
  const panel = useCanvasPanel();
  const canvases = useChatCanvases(chatId, live).data?.canvases;
  // Opened without one in mind: the newest.
  const id = panel.focus ?? canvases?.at(-1)?.id;
  const picker =
    canvases && canvases.length > 1 ? (
      <select aria-label="Canvas" value={id} onChange={(e) => panel.show(e.target.value)} className="max-w-full truncate rounded-sm border border-border bg-surface px-1.5 py-1 text-[13px] font-semibold text-fg">
        {canvases.map((c) => (
          <option key={c.id} value={c.id}>{c.title}</option>
        ))}
      </select>
    ) : null;
  return (
    <aside aria-label="Canvas" data-testid="canvas-panel" className="flex w-[48%] min-w-[420px] max-w-[760px] flex-none flex-col border-l border-border-strong bg-surface-sunken max-md:absolute max-md:inset-0 max-md:z-20 max-md:w-full max-md:min-w-0 max-md:max-w-none">
      {id ? (
        <CanvasView key={id} chatId={chatId} canvasId={id} live={live} onClose={panel.close} picker={picker} />
      ) : (
        <p className="p-4 text-xs text-fg-tertiary">This conversation has no canvases yet.</p>
      )}
    </aside>
  );
}
