"use client";

import { useQuery } from "@tanstack/react-query";
import { api, unwrap } from "./client";

// SSE `audio.updated` invalidates ["audio"] as the runner copies a render's
// progress. While a turn runs it also polls, in case an event is missed.
const LIVE_MS = 3000;

/** A chat's audio, oldest first. */
export function useChatAudio(chatId?: string, live?: boolean) {
  return useQuery({
    enabled: !!chatId,
    queryKey: ["audio", chatId ?? ""],
    queryFn: () => unwrap(api.GET("/chats/{id}/audio", { params: { path: { id: chatId! } } })),
    refetchInterval: live ? LIVE_MS : false,
  });
}
