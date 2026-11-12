"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { Icon } from "@/components/ui/icon";
import { cx } from "@/lib/cx";
import { TABS, badgeLabel, currentTab, hidesTabBar, type Tab } from "@/lib/tab-bar";
import { useNavCounts } from "@/lib/use-nav-counts";
import type { NavBadge } from "@/lib/project-scope";
import { NeedsYou } from "./needs-you";

/** The bar's height above the bottom safe-area inset; keep in step with --tab-bar-h (globals.css). */
const TAB_BAR_HEIGHT = 56;

function TabBadge({ kind, counts }: { kind?: NavBadge; counts: Record<NavBadge, number> }) {
  const n = kind ? counts[kind] : 0;
  if (!kind || n <= 0) return null;
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
        "flex min-h-[44px] flex-1 flex-col items-center justify-center gap-0.5 text-[11px] font-medium",
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
        {TABS.map((t) =>
          t.tray ? (
            <NeedsYou key={t.key} variant="tab" count={counts.inbox} />
          ) : (
            <TabLink key={t.key} tab={t} active={t.key === current} counts={counts} />
          ),
        )}
      </div>
    </nav>
  );
}
