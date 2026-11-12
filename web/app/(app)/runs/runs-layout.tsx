"use client";

// The Runs page layout (LAZYAGENTS-163): sidebar (the shell), a 320px run list,
// then the run thread and its Context panel (the page). On compact the list and
// the run are separate screens: /runs is the list, a row pushes /runs/[id].

import { usePathname } from "next/navigation";
import type { ReactNode } from "react";
import { useCompact } from "@/lib/viewport";
import { RunList } from "./run-list";

export function RunsLayout({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const compact = useCompact();
  const id = /^\/runs\/([^/]+)/.exec(pathname)?.[1];
  if (compact) return id ? <>{children}</> : <RunList fill />;
  return (
    <div className="flex h-dvh min-h-0">
      <RunList selected={id} />
      <div className="flex min-w-0 flex-1 flex-col">{children}</div>
    </div>
  );
}
