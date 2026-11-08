"use client";

import { createContext, useContext, type ReactNode } from "react";
import { useMemo } from "react";
import { VideoPlayers } from "./recommend-fence";
import { youtubeMoment } from "@/lib/chat-recommend";

/** The reply's citations: what each number is, and how to bring its source into view. */
export const CiteContext = createContext<{ titles: Map<number, string>; focus: (n: number) => void } | undefined>(undefined);

export function CiteMarker({ n }: { n: number }) {
  const cites = useContext(CiteContext);
  const title = cites?.titles.get(n);
  return (
    <button
      type="button"
      data-testid="cite"
      aria-label={`Source ${n}${title ? `: ${title}` : ""}`}
      title={title}
      onClick={() => cites?.focus(n)}
      className="mx-px inline-grid h-4 min-w-4 -translate-y-0.5 place-items-center rounded bg-accent-soft px-1 align-baseline font-mono text-[10.5px] font-semibold text-accent hover:underline"
    >
      {n}
    </button>
  );
}

/** A link to a moment in a video: jumps the chat's player there, or opens YouTube when there is none. */
export function MomentLink({ href, children }: { href: string; children: ReactNode }) {
  const players = useContext(VideoPlayers);
  const moment = useMemo(() => youtubeMoment(href), [href]);
  return (
    <a
      href={href}
      target="_blank"
      rel="noreferrer noopener"
      data-testid="moment"
      onClick={(e) => {
        const seek = moment && players?.get(moment.id);
        if (seek) {
          e.preventDefault();
          seek(moment.seconds);
        }
      }}
    >
      {children}
    </a>
  );
}
