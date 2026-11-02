import { Suspense } from "react";
import { TicketsScreen } from "../_components/tickets-screen";
import { Composer } from "./composer";

/** The composer opens as a dialog over the tickets list (lazyagents-design new-ticket.html). */
export default function NewTicketPage() {
  return (
    <Suspense>
      <TicketsScreen />
      <Composer />
    </Suspense>
  );
}
