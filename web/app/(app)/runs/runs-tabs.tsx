"use client";

import { useSearchParams } from "next/navigation";
import { Tabs } from "@/components/ui/controls";
import { CostsScreen } from "./costs-screen";
import { RunsScreen } from "./runs-screen";

export type RunsTab = "runs" | "costs";

/** /runs?tab=costs: what runs are doing, and what they cost. */
export function RunsTabs() {
  return useSearchParams().get("tab") === "costs" ? <CostsScreen /> : <RunsScreen />;
}

/** Runs · Costs, under the page heading. The project carries over; each tab's own filters don't. */
export function RunsTabNav({ tab }: { tab: RunsTab }) {
  const project = useSearchParams().get("project");
  const keep = project ? `project=${encodeURIComponent(project)}` : "";
  return (
    <Tabs
      label="Runs and costs"
      value={tab}
      items={[
        { key: "runs", label: "Runs", href: keep ? `/runs?${keep}` : "/runs" },
        { key: "costs", label: "Costs", href: `/runs?tab=costs${keep ? `&${keep}` : ""}` },
      ]}
    />
  );
}
