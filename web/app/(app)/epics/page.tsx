import { Suspense } from "react";
import { EpicsScreen } from "./epics-screen";
export default function Page() {
  return (
    <Suspense>
      <EpicsScreen />
    </Suspense>
  );
}
