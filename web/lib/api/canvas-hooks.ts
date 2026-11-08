"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { api, unwrap } from "./client";

// SSE `chat.*` invalidates ["canvases"], so a turn finishing refreshes them.
// Not under ["chats"]: sending a message awaits that prefix's refetch, and a
// slow canvas request mustn't hold up the composer. Agent edits mid-turn send
// no event, so `live` polls while a turn runs.
const LIVE_MS = 3000;

/** A chat's canvases. */
export function useChatCanvases(chatId?: string, live?: boolean) {
  return useQuery({
    enabled: !!chatId,
    queryKey: ["canvases", chatId ?? ""],
    queryFn: () => unwrap(api.GET("/chats/{id}/canvases", { params: { path: { id: chatId! } } })),
    refetchInterval: live ? LIVE_MS : false,
  });
}

/** One canvas at version `seq`, or following its current version when `seq` is undefined. */
export function useChatCanvas(chatId: string, canvasId?: string, seq?: number, live?: boolean) {
  return useQuery({
    enabled: !!canvasId,
    queryKey: ["canvases", chatId, canvasId ?? "", seq ?? "current"],
    queryFn: () => unwrap(api.GET("/chats/{id}/canvases/{canvasId}", { params: { path: { id: chatId, canvasId: canvasId! }, query: seq === undefined ? {} : { seq } } })),
    placeholderData: keepPreviousData,
    refetchInterval: live && seq === undefined ? LIVE_MS : false,
  });
}
