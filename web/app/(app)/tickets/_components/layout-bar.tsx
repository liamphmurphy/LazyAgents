"use client";

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import type { ReactNode } from "react";
import { cx } from "@/lib/cx";
import { useCompact } from "@/lib/viewport";
import { TICKETS_LAYOUTS, TICKETS_LAYOUT_LABEL, layoutSwitchHref, type TicketsLayout } from "@/lib/tickets-layout";
import { ButtonLink } from "@/components/ui/button";
import { ProjectPicker } from "@/components/shell/project-picker";

function LayoutSwitch({ layout, current, compact }: { layout: TicketsLayout; current: URLSearchParams; compact: boolean }) {
  return (
    <nav aria-label="Tickets layout" className={cx("flex rounded-lg border border-border-strong bg-surface-sunken p-0.5", compact && "w-full")}>
      {TICKETS_LAYOUTS.map((l) => (
        <Link
          key={l}
          href={layoutSwitchHref(l, current)}
          aria-current={l === layout ? "page" : undefined}
          className={cx(
            "flex items-center rounded-md px-3",
            compact ? "h-[32px] flex-1 justify-center text-[13px]" : "h-[26px] text-[12.5px]",
            l === layout ? "bg-selected font-medium text-fg shadow-[inset_0_0_0_1px_var(--border-strong)]" : "text-fg-secondary hover:text-fg",
          )}
        >
          {TICKETS_LAYOUT_LABEL[l]}
        </Link>
      ))}
    </nav>
  );
}

/**
 * Above every Tickets layout: the project, then List · Board · Epics. On compact screens it is the
 * whole header (the screens drop their top bar): the title, the project and a + button, then the switch.
 */
export function TicketsLayoutBar({ layout }: { layout: TicketsLayout }) {
  const params = useSearchParams();
  const compact = useCompact();
  const current = new URLSearchParams(params.toString());
  if (compact) {
    const project = params.get("project");
    return (
      <header className="flex flex-col gap-2.5 border-b border-border-subtle px-4 pb-2.5 pt-3">
        <div className="flex items-center gap-2">
          <h1 className="flex-1 type-heading text-[17px]">Tickets</h1>
          {/* The board shows one project at a time, so it doesn't offer All projects. */}
          <div className="flex min-w-0 max-w-[60%]">
            <ProjectPicker allowAll={layout !== "board"} />
          </div>
          <ButtonLink
            href={`/tickets/new${project ? `?project=${encodeURIComponent(project)}` : ""}`}
            variant="primary"
            icon="plus"
            aria-label="New ticket"
            className="size-[36px] flex-none justify-center px-0"
          />
        </div>
        <LayoutSwitch layout={layout} current={current} compact />
      </header>
    );
  }
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border-subtle px-6 py-2.5">
      {/* The board shows one project at a time, so it doesn't offer All projects. */}
      <ProjectPicker allowAll={layout !== "board"} />
      <LayoutSwitch layout={layout} current={current} compact={false} />
    </div>
  );
}

/** Compact screens: filter controls on one line that scrolls sideways instead of wrapping. */
export function ChipScroller({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div
      role="group"
      aria-label={label}
      data-testid="filter-chips"
      className="relative flex flex-nowrap items-center gap-2 overflow-x-auto overscroll-x-contain border-b border-border-subtle px-4 py-2 [scrollbar-width:none] [&>*]:flex-none"
    >
      {children}
    </div>
  );
}
