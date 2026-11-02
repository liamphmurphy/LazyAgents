import { Suspense } from "react";
import { WikiScreen } from "./_components/wiki-screen";

export default function WikiPage() {
  return (
    <Suspense>
      <WikiScreen path="" />
    </Suspense>
  );
}
