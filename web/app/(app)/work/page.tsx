import { Suspense } from "react";
import { WorkScreen } from "./work-screen";

export default function WorkPage() {
  return (
    <Suspense>
      <WorkScreen />
    </Suspense>
  );
}
