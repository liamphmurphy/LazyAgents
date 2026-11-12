"use client";

import Link from "next/link";
import { useLiveRuns } from "@/lib/api/hooks";
import { cx } from "@/lib/cx";
import { liveStrip, stripLabel, stripWork } from "@/lib/live-run-strip";
import { useCompact } from "@/lib/viewport";

/**
 * The compact shell's stand-in for the activity orb, one line above the chat composer: the newest
 * live run's agent, ticket and step, opening that run, and "+N more" opening Runs. Nothing without
 * a live run or on a wide screen, where the orb shows.
 */
export function LiveRunStrip() {
  const compact = useCompact();
  const live = useLiveRuns(compact);
  const s = compact ? liveStrip(live.data?.runs ?? []) : null;
  if (!s) return null;
  return (
    <div data-testid="live-run-strip" className="mb-2 flex min-h-11 items-center gap-2 rounded-lg border border-border-subtle bg-surface-sunken px-3 text-[12.5px]">
      <Link href={`/runs/${s.run.id}`} aria-label={stripLabel(s)} className="flex min-h-11 min-w-0 flex-1 items-center gap-2">
        <span aria-hidden className={cx("size-2 flex-none rounded-full", s.waiting ? "bg-state-human" : "bg-state-done motion-safe:animate-pulse")} />
        <span className="truncate">
          <b className="font-medium">{s.run.agent.name}</b>
          <span className="text-fg-tertiary"> · </span>
          <span className="font-mono">{stripWork(s.run)}</span>
          {s.step !== null && <span className="text-fg-tertiary"> · step {s.step} of {s.total}</span>}
        </span>
      </Link>
      {s.more > 0 && (
        <Link href="/runs" className="flex min-h-11 flex-none items-center text-accent">
          +{s.more} more
        </Link>
      )}
    </div>
  );
}
