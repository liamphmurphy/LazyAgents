"use client";

import { useSearchParams } from "next/navigation";
import { EpicDetailScreen } from "../epics/[id]/epic-detail-screen";
import { LegacyWork } from "./legacy-work";

export function EpicEntry({ epicId }: { epicId: string }) {
  const params = useSearchParams();
  return params.get("full") === "1" ? (
    <EpicDetailScreen epicId={epicId} />
  ) : (
    <LegacyWork view="epics" epic={epicId} />
  );
}
