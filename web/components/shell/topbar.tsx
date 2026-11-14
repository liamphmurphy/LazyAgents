import Link from "next/link";
import { Fragment, type ReactNode } from "react";

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

/** 52px top bar: breadcrumb left; actions right (secondary first, primary last). */
export function TopBar({ crumbs, actions }: { crumbs: Crumb[]; actions?: ReactNode }) {
  return (
    <header className="flex min-h-[52px] flex-wrap items-center justify-between gap-3 border-b border-border-subtle px-6 py-2.5">
      <Breadcrumb items={crumbs} />
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
