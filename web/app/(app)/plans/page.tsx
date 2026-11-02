import { Suspense } from "react";
import { PlansScreen } from "./plans-screen";
export default function Page() {
  return (
    <Suspense>
      <PlansScreen />
    </Suspense>
  );
}
