"use client";

import { useSearchParams } from "next/navigation";
import { TicketScreen } from "../tickets/[key]/ticket-screen";
import { LegacyWork } from "./legacy-work";

export function TicketEntry({ ticketKey }: { ticketKey: string }) {
  const params = useSearchParams();
  return params.get("full") === "1" ? (
    <TicketScreen ticketKey={ticketKey} />
  ) : (
    <LegacyWork view="tickets" ticket={ticketKey} />
  );
}
