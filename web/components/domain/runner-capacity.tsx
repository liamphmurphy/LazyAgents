"use client";

import type { Schemas } from "@/lib/api/client";
import { useRunnerStatus } from "@/lib/api/hooks";
import { ProgressBar } from "./data";
import { cx } from "@/lib/cx";

export function RunnerCapacity({
  runner,
}: {
  runner: Schemas["RunnerStatus"];
}) {
  const busy = Math.min(runner.busy, runner.slots);
  return (
    <div className="space-y-2 text-xs">
      <div className="flex items-center justify-between gap-2">
        <span className="text-fg-secondary">Runner capacity</span>
        <span
          className={cx(
            "font-mono",
            runner.online ? "text-fg" : "text-pill-human-fg",
          )}
        >
          {runner.online ? `${busy}/${runner.slots}` : "Offline"}
        </span>
      </div>
      <ProgressBar
        value={runner.slots ? busy / runner.slots : 0}
        label="Runner slots in use"
      />
      <p className="text-fg-tertiary">
        {runner.online
          ? `${Math.max(0, runner.slots - busy)} slots free`
          : "Start a runner to execute dispatched work"}
        {runner.queued > 0 && ` · ${runner.queued} queued`}
      </p>
    </div>
  );
}
export function SidebarCapacity() {
  const query = useRunnerStatus();
  return (
    <div className="mt-auto rounded-lg border border-border bg-surface px-3 py-3">
      {query.data ? (
        <RunnerCapacity runner={query.data} />
      ) : (
        <p className="text-xs text-fg-tertiary">
          {query.isError
            ? "Runner status unavailable"
            : "Checking runner capacity…"}
        </p>
      )}
    </div>
  );
}
