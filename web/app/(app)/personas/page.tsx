import { Suspense } from "react";
import { PersonasScreen } from "./personas-screen";

export default function Page() {
  return (
    <Suspense>
      <PersonasScreen />
    </Suspense>
  );
}
