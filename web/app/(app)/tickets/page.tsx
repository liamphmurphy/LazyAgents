import { Suspense } from "react";
import { TicketsLayouts } from "./_components/tickets-layouts";

export default function TicketsPage() {
  return (
    <Suspense>
      <TicketsLayouts />
    </Suspense>
  );
}
