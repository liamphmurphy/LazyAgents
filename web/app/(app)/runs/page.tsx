import { Suspense } from "react";
import { RunsIndex } from "./runs-index";

export default function RunsPage() {
  return (
    <Suspense>
      <RunsIndex />
    </Suspense>
  );
}
