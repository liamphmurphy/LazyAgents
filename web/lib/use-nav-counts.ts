"use client";

import { useBootstrap, useRunnerStatus, useSecrets } from "@/lib/api/hooks";
import { useScheduledTasks } from "@/lib/api/connector-hooks";
import { navCounts } from "@/lib/nav-counts";
import type { NavBadge } from "@/lib/project-scope";

/** The nav badge counts. SSE invalidates ["bootstrap"] and ["runner"] (live-updates.tsx), so they stay current. */
export function useNavCounts(): Record<NavBadge, number> {
  const boot = useBootstrap();
  const runner = useRunnerStatus();
  const scheduled = useScheduledTasks(true);
  const secrets = useSecrets();
  return navCounts({ bootstrap: boot.data, runner: runner.data, scheduled: scheduled.data?.tasks, secrets: secrets.data });
}
