import { Suspense } from "react";
import { LegacyWork } from "../work/legacy-work";

export default function TicketsPage() {
  return (
    <Suspense>
      <LegacyWork view="tickets" />
    </Suspense>
  );
}
