import { Suspense } from "react";
import { RunsScreen } from "./runs-screen";

export default function RunsPage() {
  return (
    <Suspense>
      <RunsScreen />
    </Suspense>
  );
}
