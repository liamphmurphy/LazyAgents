"use client";

import { useQuery } from "@tanstack/react-query";
import { api, unwrap } from "./client";

// SSE `video.updated` invalidates ["videos"] as the runner copies a render's
// progress. While a turn runs it also polls, in case an event is missed.
const LIVE_MS = 3000;

/** A chat's videos, oldest first. */
export function useChatVideos(chatId?: string, live?: boolean) {
  return useQuery({
    enabled: !!chatId,
    queryKey: ["videos", chatId ?? ""],
    queryFn: () => unwrap(api.GET("/chats/{id}/videos", { params: { path: { id: chatId! } } })),
    refetchInterval: live ? LIVE_MS : false,
  });
}
