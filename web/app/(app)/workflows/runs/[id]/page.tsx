import { Suspense } from "react";
import { WorkflowRunScreen } from "../../_components/run-screen";

export default function Page() {
  return (
    <Suspense>
      <WorkflowRunScreen />
    </Suspense>
  );
}
