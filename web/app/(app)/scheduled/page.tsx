import { Suspense } from "react";
import { ScheduledScreen } from "./scheduled-screen";

export default function Page() {
  return (
    <Suspense>
      <ScheduledScreen />
    </Suspense>
  );
}
