"use client";

import { useState } from "react";
import { langFromFence } from "@/lib/code-diff";
import { useHighlighted } from "@/lib/highlight";
import { Icon } from "@/components/ui/icon";

export const FENCE_COLLAPSE_LINES = 20;

/** A fenced code block in chat markdown: language header, Copy, lazy syntax highlighting, long blocks collapsed. */
export function CodeFence({ code, info }: { code: string; info?: string }) {
  const lines = code.replace(/\n$/, "").split("\n");
  const lang = langFromFence(info);
  const label = info?.trim().split(/\s+/)[0] || "text";
  const [expanded, setExpanded] = useState(false);
  const [copied, setCopied] = useState(false);
  const long = lines.length > FENCE_COLLAPSE_LINES;
  const shown = long && !expanded ? lines.slice(0, FENCE_COLLAPSE_LINES) : lines;
  const toks = useHighlighted([shown], lang, true)?.[0];
  return (
    <div className="overflow-hidden rounded-lg border border-border bg-surface-sunken" data-testid="code-fence">
      <div className="flex items-center justify-between border-b border-border-row px-3 py-1 text-xs text-fg-tertiary">
        <span className="font-mono">{label}</span>
        <button
          type="button"
          className="flex items-center gap-1 rounded px-1 hover:text-fg"
          onClick={() => {
            void navigator.clipboard?.writeText(lines.join("\n")).then(() => {
              setCopied(true);
              setTimeout(() => setCopied(false), 1500);
            });
          }}
        >
          <Icon name={copied ? "check" : "link"} size={12} />
          {copied ? "Copied" : "Copy"}
        </button>
      </div>
      <pre className="overflow-x-auto p-3 font-mono text-xs leading-relaxed">
        <code>
          {shown.map((l, i) => (
            <span key={i} className="block whitespace-pre">
              {toks?.[i]
                ? toks[i].map((t, j) => (
                    <span key={j} className="shiki-tok" style={t.style}>
                      {t.content}
                    </span>
                  ))
                : l}
              {l === "" && " "}
            </span>
          ))}
        </code>
      </pre>
      {long && (
        <button
          type="button"
          aria-expanded={expanded}
          className="w-full border-t border-border-row px-3 py-1 text-left text-xs text-fg-secondary hover:text-fg"
          onClick={() => setExpanded((e) => !e)}
        >
          {expanded ? "Show less" : `Show ${lines.length - FENCE_COLLAPSE_LINES} more lines`}
        </button>
      )}
    </div>
  );
}
