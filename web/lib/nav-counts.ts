/**
 * The nav badge counts (Needs you, Runs, Scheduled, Wiki, Settings), shared by the
 * sidebar and the mobile tab bar. Pure so each source is unit-tested; the
 * useNavCounts hook (use-nav-counts.ts) feeds it from the API queries.
 */
import type { NavBadge } from "./project-scope.ts";
import { tasksNeedingYou } from "./schedule-ui.ts";
import { needsAttention, type SecretMeta } from "./settings.ts";

export type NavCountSources = {
  /** From bootstrap: open items the person has to act on (Needs you) and agent wiki edits waiting for a person. */
  bootstrap?: { counts: { inbox: number; needs_you?: number; wiki_suggestions?: number } };
  /** From the runner status: ticket and planning runs in progress (chat runs aren't counted). */
  runner?: { live?: number };
  scheduled?: { enabled: boolean; runs: { state: string }[] }[];
  secrets?: { key_configured: boolean; secrets: SecretMeta[] };
};

export function navCounts({ bootstrap, runner, scheduled, secrets }: NavCountSources): Record<NavBadge, number> {
  return {
    // The Needs you badge: Updates (news that asks nothing) are never counted.
    inbox: bootstrap?.counts.needs_you ?? 0,
    runs: runner?.live ?? 0,
    scheduled: tasksNeedingYou(scheduled),
    wiki: bootstrap?.counts.wiki_suggestions ?? 0,
    settings: needsAttention(secrets) ? 1 : 0,
  };
}
