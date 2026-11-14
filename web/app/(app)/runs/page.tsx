import { Suspense } from "react";
import { LegacyWork } from "../work/legacy-work";

export default function RunsPage() {
  return (
    <Suspense>
      <LegacyWork view="activity" />
    </Suspense>
  );
}
