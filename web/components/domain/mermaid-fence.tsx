"use client";

import { useEffect, useId, useState } from "react";
import { svgDataUrl } from "@/lib/chat-svg";
import { CodeFence } from "./code-fence";

export const isMermaidFence = (info: string | undefined) => (info ?? "").trim().split(/\s+/)[0]?.toLowerCase() === "mermaid";

const MAX_SOURCE = 20_000;

/**
 * A ```mermaid fence, drawn as a diagram. The result shows as an image, like svg-image
 * fences, so nothing in a reply's diagram can run. Mermaid loads only when one appears.
 */
export function MermaidFence({ code, info }: { code: string; info?: string }) {
  const id = `mermaid-${useId().replace(/[^a-zA-Z0-9]/g, "")}`;
  const [drawn, setDrawn] = useState<{ code: string; svg?: string }>();
  const [showCode, setShowCode] = useState(false);
  useEffect(() => {
    if (code.length > MAX_SOURCE) return;
    let live = true;
    import("mermaid")
      .then(async ({ default: mermaid }) => {
        mermaid.initialize({ startOnLoad: false, securityLevel: "strict", suppressErrorRendering: true, htmlLabels: false, flowchart: { htmlLabels: false, useMaxWidth: false }, sequence: { useMaxWidth: false } });
        const { svg } = await mermaid.render(id, code);
        if (live) setDrawn({ code, svg });
      })
      .catch(() => {
        if (live) setDrawn({ code });
      });
    return () => {
      live = false;
    };
  }, [code, id]);
  const current = drawn?.code === code ? drawn : undefined;
  if (code.length > MAX_SOURCE || (current && !current.svg)) return <CodeFence code={code} info={info} />;
  if (!current) return <div data-testid="mermaid-loading" aria-label="Drawing diagram" className="h-24 w-full animate-pulse rounded-lg border border-border bg-surface-sunken" />;
  return (
    <div data-testid="mermaid" className="flex max-w-full flex-col gap-1">
      {showCode ? (
        <CodeFence code={code} info={info} />
      ) : (
        // eslint-disable-next-line @next/next/no-img-element
        <img src={svgDataUrl(current.svg!)} alt="Diagram" className="max-h-[480px] w-auto max-w-full self-start rounded-lg border border-border bg-white object-contain p-2" />
      )}
      <button type="button" onClick={() => setShowCode((v) => !v)} className="self-start text-xs text-fg-tertiary hover:text-fg">{showCode ? "Show diagram" : "Show code"}</button>
    </div>
  );
}
