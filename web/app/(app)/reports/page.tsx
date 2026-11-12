import { Suspense } from "react";
import { ReportsScreen } from "./reports-tabs";

export default function ReportsPage() {
  return (
    <Suspense>
      <ReportsScreen />
    </Suspense>
  );
}
