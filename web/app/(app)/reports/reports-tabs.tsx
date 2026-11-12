"use client";

import { useSearchParams } from "next/navigation";
import { Tabs } from "@/components/ui/controls";
import { CostsScreen } from "./costs-screen";
import { HighlightsScreen } from "./highlights-screen";

export type ReportsTab = "costs" | "highlights";

/** Reports (LAZYAGENTS-163): recorded spend and private persona routing decisions, moved here from the Runs tabs. */
export function ReportsScreen() {
  return useSearchParams().get("tab") === "highlights" ? <HighlightsScreen /> : <CostsScreen />;
}

/** The project carries over; each tab's own filters don't. */
export function ReportsTabNav({ tab }: { tab: ReportsTab }) {
  const project = useSearchParams().get("project");
  const keep = project ? `&project=${encodeURIComponent(project)}` : "";
  return (
    <Tabs
      label="Reports"
      value={tab}
      items={[
        { key: "costs", label: "Costs", href: `/reports?tab=costs${keep}` },
        { key: "highlights", label: "Highlights", href: `/reports?tab=highlights${keep}` },
      ]}
    />
  );
}
