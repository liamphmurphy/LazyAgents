"use client";

import { useSearchParams } from "next/navigation";
import { ticketsLayout } from "@/lib/tickets-layout";
import { BoardScreen } from "./board-screen";
import { EpicsScreen } from "./epics-screen";
import { TicketsScreen } from "./tickets-screen";

/** /tickets?layout=: the list (default), the board, or the epics. */
export function TicketsLayouts() {
  const layout = ticketsLayout(useSearchParams().get("layout"));
  if (layout === "board") return <BoardScreen />;
  if (layout === "epics") return <EpicsScreen />;
  return <TicketsScreen />;
}
