"use client";

import { useRouter, useSelectedLayoutSegment } from "next/navigation";
import { Suspense, useEffect, type ReactNode } from "react";
import { useBootstrap } from "@/lib/api/hooks";
import { Skeleton } from "@/components/ui/primitives";
import { LiveUpdates } from "./live-updates";
import { ActivityOrb } from "./activity-orb";
import { CommandMenu } from "./command-menu";
import { isTyping, ShortcutsDialog } from "./shortcuts";
import { Sidebar } from "./sidebar";
import { SystemBanner } from "./system-banner";

/** Global single-key shortcuts (DESIGN.md §6). */
function useShortcuts() {
  const router = useRouter();
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (
        e.metaKey ||
        e.ctrlKey ||
        e.altKey ||
        isTyping(e.target) ||
        document.querySelector('[role="dialog"]')
      )
        return;
      if (e.key === "?") {
        e.preventDefault();
        window.dispatchEvent(new Event("la:shortcuts"));
        return;
      }
      if (e.key === "c" || e.key === "C") {
        e.preventDefault();
        // Keep the project in view as the composer's default.
        const project = new URLSearchParams(window.location.search).get(
          "project",
        );
        router.push(
          `/tickets/new${project ? `?project=${encodeURIComponent(project)}` : ""}`,
        );
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [router]);
}

function SidebarSkeleton() {
  return (
    // Matches the menu's collapsed default (an icon rail).
    <aside
      aria-busy="true"
      className="flex w-[52px] flex-none flex-col items-center gap-4 border-r border-border-subtle bg-sidebar px-1.5 pb-3.5 pt-3"
    >
      {[0, 1, 2, 3, 4, 5, 6].map((i) => (
        <Skeleton key={i} className="size-[26px]" />
      ))}
    </aside>
  );
}

const PAGE = "flex min-h-0 min-w-0 flex-1 flex-col";

/**
 * Fades in a new section; navigation within one stays instant. CSS rather than <ViewTransition>:
 * a view transition around the page drops clicks and shortcuts made while it plays.
 */
function SectionFade({ children }: { children: ReactNode }) {
  const section = useSelectedLayoutSegment();
  return <div key={section} className={`motion-page ${PAGE}`}>{children}</div>;
}

/**
 * Wrapping flex row: below ~800px the sidebar stacks above the content
 * (DESIGN.md §3 Responsive).
 */
export function AppShell({ children }: { children: ReactNode }) {
  const { data } = useBootstrap();
  useShortcuts();
  return (
    <div className="flex min-h-dvh flex-wrap bg-bg text-fg">
      <LiveUpdates />
      {/* usePathname needs a Suspense boundary for prerendering. */}
      <Suspense fallback={null}>
        <CommandMenu />
      </Suspense>
      <ShortcutsDialog />
      <Suspense fallback={null}>
        <ActivityOrb />
      </Suspense>
      {data ? (
        <Suspense fallback={<SidebarSkeleton />}>
          <Sidebar />
        </Suspense>
      ) : (
        <SidebarSkeleton />
      )}
      <main className="flex min-w-0 flex-[999_1_560px] flex-col">
        <SystemBanner />
        {/* The segment needs a Suspense boundary for prerendering; the page still paints meanwhile. */}
        <Suspense fallback={<div className={PAGE}>{children}</div>}>
          <SectionFade>{children}</SectionFade>
        </Suspense>
      </main>
    </div>
  );
}
