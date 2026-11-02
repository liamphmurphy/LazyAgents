import { Suspense } from "react";
import { TicketsScreen } from "./_components/tickets-screen";

export default function TicketsPage() {
  return (
    <Suspense>
      <TicketsScreen />
    </Suspense>
  );
}
