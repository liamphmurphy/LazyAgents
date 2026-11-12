"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useEffect } from "react";
import { useCompact } from "@/lib/viewport";
import { EmptyState, Skeleton } from "@/components/ui/primitives";
import { listQuery, useRunList } from "./run-list";

/**
 * /runs on desktop opens the list's first row (the first Needs you run, else the
 * newest live run) in place, so the URL always names the run beside the list.
 * On compact the layout shows the list alone and this renders nothing.
 */
export function RunsIndex() {
  const compact = useCompact();
  const router = useRouter();
  const params = useSearchParams();
  const { first, loading } = useRunList();
  const query = listQuery(params);
  useEffect(() => {
    if (compact || !first) return;
    router.replace(`/runs/${first}${query ? `?${query}` : ""}${window.location.hash}`, { scroll: false });
  }, [compact, first, query, router]);
  if (compact) return null;
  if (loading || first) {
    return (
      <div aria-busy="true" className="flex flex-col gap-3 p-6">
        <Skeleton className="h-10 w-full" />
        <Skeleton className="h-3 w-2/3" />
      </div>
    );
  }
  return (
    <div className="grid flex-1 place-items-center p-6">
      <EmptyState icon="play" title="No runs here">Start a run from a ticket or a chat, or change the filters.</EmptyState>
    </div>
  );
}
