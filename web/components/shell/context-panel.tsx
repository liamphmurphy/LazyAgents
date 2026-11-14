// A titled section of a companion dock's facts tab: the run's and the epic's Context
// (LAZYAGENTS-163, LAZYAGENTS-171). The panel itself is the companion (components/shell/companion.tsx).

import type { ReactNode } from "react";

export function ContextSection({ title, aside, children }: { title: string; aside?: ReactNode; children: ReactNode }) {
  return (
    <section aria-label={title} className="flex flex-col gap-2 border-b border-border-subtle px-4 py-3.5 last:border-b-0">
      <h2 className="flex items-center justify-between text-12 font-medium text-fg-tertiary">
        {title}
        {aside && <span className="font-mono">{aside}</span>}
      </h2>
      {children}
    </section>
  );
}
