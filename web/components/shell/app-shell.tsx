"use client";

import { useRouter } from "next/navigation";
import { Suspense, useEffect, type ReactNode } from "react";
import { useBootstrap, useRunnerStatus } from "@/lib/api/hooks";
import { Skeleton } from "@/components/ui/primitives";
import { LiveUpdates } from "./live-updates";
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
    <aside
      aria-busy="true"
      className="flex flex-[1_1_232px] flex-col gap-5 border-r border-border-subtle bg-sidebar px-2.5 pb-3.5 pt-3"
    >
      <Skeleton className="h-8 w-40" />
      <Skeleton className="h-8" />
      <div className="flex flex-col gap-3 px-2">
        {[70, 50, 60, 55, 60, 65].map((w, i) => (
          <Skeleton key={i} className="h-3" style={{ width: `${w}%` }} />
        ))}
      </div>
    </aside>
  );
}

/**
 * Wrapping flex row: below ~800px the sidebar stacks above the content
 * (DESIGN.md §3 Responsive).
 */
export function AppShell({ children }: { children: ReactNode }) {
  const { data } = useBootstrap();
  // Inbox and Runs counts: SSE invalidates ["bootstrap"] and ["runner"] (live-updates.tsx).
  const runner = useRunnerStatus();
  useShortcuts();
  return (
    <div className="flex min-h-dvh flex-wrap bg-bg text-fg">
      <LiveUpdates />
      {/* usePathname needs a Suspense boundary for prerendering. */}
      <Suspense fallback={null}>
        <CommandMenu />
      </Suspense>
      <ShortcutsDialog />
      {data ? (
        <Suspense fallback={<SidebarSkeleton />}>
          <Sidebar
            workspace={data.workspace.name}
            inboxCount={data.counts.inbox}
            liveRuns={runner.data?.busy ?? 0}
            wikiSuggestions={data.counts.wiki_suggestions}
            projects={data.projects}
            user={{
              name: data.me.name,
              initials: data.me.initials,
              role: data.me.role_title,
            }}
          />
        </Suspense>
      ) : (
        <SidebarSkeleton />
      )}
      <main className="flex min-w-0 flex-[999_1_560px] flex-col">
        <SystemBanner />
        {children}
      </main>
    </div>
  );
}
