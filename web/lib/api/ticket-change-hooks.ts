"use client";

// Ticket edits a persona proposed in a chat (update_ticket, comment_ticket,
// update_ticket_blockers), waiting for the person to apply or decline them.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, type Schemas } from "./client";
import { keys } from "./hooks";

export type TicketChangeRequest = Schemas["TicketChangeRequest"];
export type TicketChangeDecision = Schemas["TicketChangeDecision"]["decision"];

const changesKey = (chatId: string) => ["ticket-changes", chatId] as const;

/** Polled: a proposal arrives from a persona's tool call mid-turn, with no event of its own. */
export function useTicketChangeRequests(chatId: string) {
  return useQuery({
    queryKey: changesKey(chatId),
    refetchInterval: 3000,
    queryFn: () => unwrap(api.GET("/chats/{id}/ticket-changes", { params: { path: { id: chatId } } })),
  });
}

/** Applying changes a ticket, and "always" changes the persona, so both refetch. */
export function useDecideTicketChange(chatId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { id: string; decision: TicketChangeDecision }) =>
      unwrap(api.POST("/ticket-changes/{id}", { params: { path: { id: v.id } }, body: { decision: v.decision } })),
    onSettled: (_data, _err, v) =>
      Promise.all([
        qc.invalidateQueries({ queryKey: changesKey(chatId) }),
        qc.invalidateQueries({ queryKey: keys.tickets }),
        v.decision === "always" ? qc.invalidateQueries({ queryKey: ["personas"] }) : null,
      ]),
  });
}
