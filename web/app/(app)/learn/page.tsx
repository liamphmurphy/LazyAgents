import { Suspense } from "react";
import { LearnScreen } from "./_components/learn-screen";

export default function Page() {
  return (
    <Suspense>
      <LearnScreen />
    </Suspense>
  );
}
