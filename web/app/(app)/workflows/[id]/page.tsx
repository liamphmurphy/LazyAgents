import { Suspense } from "react";
import { WorkflowEditorScreen } from "../_components/editor";

export default function Page() {
  return (
    <Suspense>
      <WorkflowEditorScreen />
    </Suspense>
  );
}
