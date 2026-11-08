"use client";

import { useMemo } from "react";
import { Markdown } from "@/components/domain/markdown";
import { SvgFenceMessage } from "@/components/domain/svg-fence";

const TICKET_KEY = /\b([A-Z][A-Z0-9]+-\d+)\b/g;
const FENCE = /^ {0,3}(```|~~~)/;
// Code spans and existing [[links]] stay untouched.
const SKIP = /(`+[^`\n]*`+|!?\[\[[^[\]\n]+?\]\])/g;

/** Wraps bare ticket keys (LAZYAGENTS-12) in [[…]] so the shared renderer links them. */
export function linkTicketKeys(md: string): string {
  let fence = "";
  return md
    .split("\n")
    .map((line) => {
      const f = FENCE.exec(line);
      if (f) {
        if (!fence) fence = f[1];
        else if (fence === f[1]) fence = "";
        return line;
      }
      if (fence) return line;
      return line
        .split(SKIP)
        .map((part, i) => (i % 2 ? part : part.replace(TICKET_KEY, "[[$1]]")))
        .join("");
    })
    .join("\n");
}

/** replyId: the assistant reply this is, so its SVG diagrams can be saved. */
export function ChatMarkdown({ children, replyId }: { children: string; replyId?: string }) {
  const md = useMemo(() => linkTicketKeys(children), [children]);
  return (
    <SvgFenceMessage.Provider value={replyId}>
      <Markdown codeBlocks>{md}</Markdown>
    </SvgFenceMessage.Provider>
  );
}
