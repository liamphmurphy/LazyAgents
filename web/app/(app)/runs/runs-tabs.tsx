"use client";

import { useSearchParams } from "next/navigation";
import { Tabs } from "@/components/ui/controls";
import { HighlightsScreen } from "./highlights-screen";
import { CostsScreen } from "./costs-screen";
import { RunsScreen } from "./runs-screen";

export type RunsTab = "runs" | "costs" | "highlights";

/** Run activity, recorded spend, and private persona routing decisions. */
export function RunsTabs() {
  const tab = useSearchParams().get("tab");
  return tab === "highlights" ? <HighlightsScreen /> : tab === "costs" ? <CostsScreen /> : <RunsScreen />;
}

/** The project carries over; each tab's own filters don't. */
export function RunsTabNav({ tab }: { tab: RunsTab }) {
  const project = useSearchParams().get("project");
  const keep = project ? `project=${encodeURIComponent(project)}` : "";
  return (
    <Tabs
      label="Run reports"
      value={tab}
      items={[
        { key: "runs", label: "Runs", href: keep ? `/runs?${keep}` : "/runs" },
        { key: "costs", label: "Costs", href: `/runs?tab=costs${keep ? `&${keep}` : ""}` },
        { key: "highlights", label: "Highlights", href: `/runs?tab=highlights${keep ? `&${keep}` : ""}` },
      ]}
    />
  );
}
