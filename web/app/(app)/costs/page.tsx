import { Suspense } from "react";
import { CostsScreen } from "./costs-screen";

export default function CostsPage() {
  return (
    <Suspense>
      <CostsScreen />
    </Suspense>
  );
}
