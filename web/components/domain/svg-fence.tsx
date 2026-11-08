"use client";

import { createContext, useContext, useMemo, useState } from "react";
import { useSaveLibraryDiagram } from "@/lib/api/library-hooks";
import { svgDataUrl, svgDocument } from "@/lib/chat-svg";
import { cx } from "@/lib/cx";
import { Icon } from "@/components/ui/icon";
import { CodeFence } from "./code-fence";

/** The assistant reply a fence is in; without one there's no "Save to Library". */
export const SvgFenceMessage = createContext<string | undefined>(undefined);

/** Unlike generated images, diagrams reach the Library only from here. */
function SaveToLibrary({ messageId, svg }: { messageId: string; svg: string }) {
  const save = useSaveLibraryDiagram();
  const saved = save.isSuccess && save.variables.svg === svg;
  return (
    <>
      {save.isError && <span role="alert" className="text-pill-blocked-fg">{save.error.message || "Couldn’t save."}</span>}
      <button type="button" disabled={save.isPending || saved} className="flex items-center gap-1 rounded px-1 hover:text-fg disabled:hover:text-fg-tertiary" onClick={() => save.mutate({ message_id: messageId, svg })}>
        <Icon name={saved ? "check" : "bookmark"} size={12} />
        {saved ? "Saved" : save.isPending ? "Saving…" : "Save to Library"}
      </button>
    </>
  );
}

/**
 * An ```svg-image fence in chat, shown through an <img> so the agent's markup
 * can't script or load anything (see svgDataUrl). Code shows the source; a
 * document the browser can't draw falls back to it.
 */
export function SvgFence({ code, info }: { code: string; info?: string }) {
  const [view, setView] = useState<"image" | "code">("image");
  const url = useMemo(() => svgDataUrl(code), [code]);
  // By URL, so an edited source gets another try.
  const [failedUrl, setFailedUrl] = useState<string>();
  const failed = failedUrl === url;
  const messageId = useContext(SvgFenceMessage);

  const tab = (v: "image" | "code", label: string) => (
    <button
      type="button"
      role="tab"
      aria-selected={view === v}
      className={cx("rounded px-1.5", view === v ? "bg-surface text-fg" : "hover:text-fg")}
      onClick={() => setView(v)}
    >
      {label}
    </button>
  );

  return (
    <div className="overflow-hidden rounded-lg border border-border bg-surface-sunken" data-testid="svg-fence">
      <div className="flex items-center justify-between gap-2 border-b border-border-row px-3 py-1 text-xs text-fg-tertiary">
        <span className="flex items-center gap-1.5">
          <Icon name="library" size={12} />
          <span>SVG image</span>
        </span>
        <span className="flex items-center gap-1">
          {!failed && (
            <span role="tablist" aria-label="Image view" className="flex items-center gap-0.5">
              {tab("image", "Image")}
              {tab("code", "Code")}
            </span>
          )}
          {!failed && messageId && <SaveToLibrary messageId={messageId} svg={svgDocument(code)} />}
          {!failed && (
            <a href={url} download="image.svg" aria-label="Download SVG" title="Download" className="flex items-center rounded px-1 hover:text-fg">
              <Icon name="download" size={12} />
            </a>
          )}
        </span>
      </div>
      {view === "image" && !failed ? (
        <div className="overflow-auto bg-white p-2">
          {/* eslint-disable-next-line @next/next/no-img-element -- a data: URL; next/image adds nothing here */}
          <img src={url} alt="SVG image" className="mx-auto block h-auto max-h-[720px] max-w-full" onError={() => setFailedUrl(url)} />
        </div>
      ) : (
        <>
          {failed && <p role="alert" className="border-b border-border-row px-3 py-1 text-xs text-pill-blocked-fg">This SVG couldn&apos;t be displayed. Its source is below.</p>}
          {/* The inner fence sits flush inside this frame's border. */}
          <div className="[&>div]:rounded-none [&>div]:border-0">
            <CodeFence code={code} info={info?.replace(/^\s*\S+/, "html")} />
          </div>
        </>
      )}
    </div>
  );
}
