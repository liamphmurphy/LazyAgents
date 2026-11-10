import { Suspense } from "react";
import { RunsTabs } from "./runs-tabs";

export default function RunsPage() {
  return (
    <Suspense>
      <RunsTabs />
    </Suspense>
  );
}
