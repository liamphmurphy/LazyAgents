"use client";

import { useSearchParams } from "next/navigation";
import { RunLogScreen } from "../runs/[id]/run-log";
import { LegacyWork } from "./legacy-work";

export function RunEntry({ id }: { id: string }) {
  const params = useSearchParams();
  const mode = params.get("view");
  // Raw log, Graph and Files keep their existing URLs, including event anchors.
  if (mode === "details" || mode === "graph" || mode === "files")
    return <RunLogScreen id={id} />;
  return <LegacyWork view="activity" run={id} />;
}
