import Link from "next/link";
import { Fragment, type ReactNode } from "react";
import { HistoryNav } from "./history-nav";

export type Crumb = { label: ReactNode; href?: string; swatch?: string; mono?: boolean };

export function Breadcrumb({ items }: { items: Crumb[] }) {
  return (
    <nav aria-label="Breadcrumb" className="flex flex-wrap items-center gap-2 text-13 text-fg-tertiary">
      {items.map((c, i) => {
        const last = i === items.length - 1;
        if (last) {
          return (
            <span key={i} aria-current="page" className={c.mono ? "font-mono text-13 text-fg" : "font-medium text-fg"}>
              {c.label}
            </span>
          );
        }
        const inner = (
          <>
            {c.swatch && <span aria-hidden className="size-2.5 rounded-[3px]" style={{ background: c.swatch }} />}
            {c.label}
          </>
        );
        return (
          <Fragment key={i}>
            {c.href ? (
              <Link href={c.href} className="flex items-center gap-2 text-fg-2 hover:text-fg">
                {inner}
              </Link>
            ) : (
              <span className="flex items-center gap-2">{inner}</span>
            )}
            <span aria-hidden>/</span>
          </Fragment>
        );
      })}
    </nav>
  );
}

/** A pane's 44px bar: back/forward and the breadcrumb left; actions right (secondary first, primary last). */
export function TopBar({ crumbs, actions }: { crumbs: Crumb[]; actions?: ReactNode }) {
  return (
    <header className="flex min-h-[44px] flex-wrap items-center justify-between gap-3 border-b border-border-subtle py-1.5 pl-3 pr-6 max-md:pl-6">
      <div className="flex min-w-0 items-center gap-2">
        <HistoryNav className="max-md:hidden" />
        <Breadcrumb items={crumbs} />
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
    </header>
  );
}

/** Page body container: max 1180, padding 32/28 (DESIGN.md §3). */
export function Content({ children, wide }: { children: ReactNode; wide?: boolean }) {
  return (
    <div className={wide ? "w-full px-7 py-8" : "mx-auto w-full max-w-[var(--content-max)] px-7 py-8 max-sm:px-4"}>{children}</div>
  );
}
