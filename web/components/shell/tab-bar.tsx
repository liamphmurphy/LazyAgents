"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { Icon } from "@/components/ui/icon";
import { usePulse } from "@/components/ui/motion";
import { cx } from "@/lib/cx";
import { TABS, badgeLabel, currentTab, hidesTabBar, type Tab } from "@/lib/tab-bar";
import { useNavCounts } from "@/lib/use-nav-counts";
import type { NavBadge } from "@/lib/project-scope";

/** The bar's height above the bottom safe-area inset; keep in step with --tab-bar-h (globals.css). */
const TAB_BAR_HEIGHT = 56;

function TabBadge({ kind, counts }: { kind?: NavBadge; counts: Record<NavBadge, number> }) {
  const n = kind ? counts[kind] : 0;
  const pulse = usePulse<HTMLSpanElement>(n);
  if (!kind || n <= 0) return null;
  if (kind === "inbox") {
    return (
      <span
        ref={pulse}
        aria-hidden
        data-testid="tab-badge"
        className="absolute -right-1 -top-1 grid h-4 min-w-4 place-items-center rounded-full bg-state-human px-1 text-[10.5px] font-semibold leading-none text-approve-fg"
      >
        {n > 99 ? "99+" : n}
      </span>
    );
  }
  if (kind === "runs") return <span aria-hidden data-testid="tab-dot" className="absolute right-1.5 top-0.5 size-1.5 rounded-full bg-state-done" />;
  return null;
}

function TabLink({ tab, active, counts }: { tab: Tab; active: boolean; counts: Record<NavBadge, number> }) {
  return (
    <Link
      href={tab.href}
      aria-current={active ? "page" : undefined}
      aria-label={badgeLabel(tab.label, tab.badge, counts)}
      className={cx(
        "flex min-h-11 flex-1 flex-col items-center justify-center gap-0.5 text-[11px] font-medium",
        active ? "text-fg" : "text-fg-tertiary",
      )}
    >
      <span className={cx("relative grid h-7 w-12 place-items-center rounded-full", active && "bg-selected")}>
        <Icon name={tab.icon} size={18} strokeWidth={1.6} />
        <TabBadge kind={tab.badge} counts={counts} />
      </span>
      {tab.label}
    </Link>
  );
}

/**
 * The compact shell's navigation (DESIGN.md §3 Responsive): fixed to the bottom, padded for the
 * home indicator. Detail screens hide it. While it shows, --tab-bar-h reserves its height.
 */
export function TabBar() {
  const pathname = usePathname();
  const counts = useNavCounts();
  if (hidesTabBar(pathname)) return null;
  const current = currentTab(pathname);
  return (
    <nav
      aria-label="Primary"
      data-tab-bar
      className="fixed inset-x-0 bottom-0 z-30 border-t border-border-subtle bg-sidebar px-safe pb-safe"
    >
      <div className="flex items-stretch px-1" style={{ height: TAB_BAR_HEIGHT }}>
        {TABS.map((t) => (
          <TabLink key={t.key} tab={t} active={t.key === current} counts={counts} />
        ))}
      </div>
    </nav>
  );
}
