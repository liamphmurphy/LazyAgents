"use client";

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { cx } from "@/lib/cx";
import { TICKETS_LAYOUTS, TICKETS_LAYOUT_LABEL, layoutSwitchHref, type TicketsLayout } from "@/lib/tickets-layout";
import { ProjectPicker } from "@/components/shell/project-picker";

/** Above every Tickets layout: the project, then List · Board · Epics. */
export function TicketsLayoutBar({ layout }: { layout: TicketsLayout }) {
  const params = useSearchParams();
  const current = new URLSearchParams(params.toString());
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border-subtle px-6 py-2.5">
      {/* The board shows one project at a time, so it doesn't offer All projects. */}
      <ProjectPicker allowAll={layout !== "board"} />
      <nav aria-label="Tickets layout" className="flex rounded-lg border border-border-strong bg-surface-sunken p-0.5">
        {TICKETS_LAYOUTS.map((l) => (
          <Link
            key={l}
            href={layoutSwitchHref(l, current)}
            aria-current={l === layout ? "page" : undefined}
            className={cx(
              "flex h-[26px] items-center rounded-md px-3 text-[12.5px]",
              l === layout ? "bg-selected font-medium text-fg shadow-[inset_0_0_0_1px_var(--border-strong)]" : "text-fg-secondary hover:text-fg",
            )}
          >
            {TICKETS_LAYOUT_LABEL[l]}
          </Link>
        ))}
      </nav>
    </div>
  );
}
