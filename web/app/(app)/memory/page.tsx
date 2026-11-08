import { Suspense } from "react";
import { MemoryScreen } from "./memory-screen";

export default function Page() {
  return (
    <Suspense>
      <MemoryScreen />
    </Suspense>
  );
}
