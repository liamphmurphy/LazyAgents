"use client";

import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, type Schemas } from "./client";

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

/** A canvas's comments and replies. */
export function useCanvasComments(chatId: string, canvasId?: string, live?: boolean) {
  return useQuery({
    enabled: !!canvasId,
    queryKey: ["canvases", chatId, canvasId ?? "", "comments"],
    queryFn: () => unwrap(api.GET("/chats/{id}/canvases/{canvasId}/comments", { params: { path: { id: chatId, canvasId: canvasId! } } })),
    refetchInterval: live ? LIVE_MS : false,
  });
}

/** Everything a person can do to a canvas. Refreshes aren't awaited, so a slow one can't hold a button. */
export function useCanvasActions(chatId: string, canvasId: string) {
  const qc = useQueryClient();
  const path = { id: chatId, canvasId };
  const refresh = () => void qc.invalidateQueries({ queryKey: ["canvases"] });
  const add = useMutation({
    mutationFn: (body: Schemas["NewChatCanvasComment"]) => unwrap(api.POST("/chats/{id}/canvases/{canvasId}/comments", { params: { path }, body })),
    onSettled: refresh,
  });
  const send = useMutation({
    mutationFn: async (body: { comment_ids: string[]; message?: string }) => {
      const { error } = await api.POST("/chats/{id}/canvases/{canvasId}/comments/send", { params: { path }, body });
      if (error) throw new Error(error.message);
    },
    onSettled: () => {
      refresh();
      void qc.invalidateQueries({ queryKey: ["chats"] });
    },
  });
  const setStatus = useMutation({
    mutationFn: (v: { id: string; status: string }) => unwrap(api.PATCH("/chats/{id}/canvases/{canvasId}/comments/{commentId}", { params: { path: { ...path, commentId: v.id } }, body: { status: v.status } })),
    onSettled: refresh,
  });
  const remove = useMutation({
    mutationFn: async (commentId: string) => {
      const { error } = await api.DELETE("/chats/{id}/canvases/{canvasId}/comments/{commentId}", { params: { path: { ...path, commentId } } });
      if (error) throw new Error(error.message);
    },
    onSettled: refresh,
  });
  const apply = useMutation({
    mutationFn: (commentId: string) => unwrap(api.POST("/chats/{id}/canvases/{canvasId}/comments/{commentId}/apply", { params: { path: { ...path, commentId } } })),
    onSettled: refresh,
  });
  const save = useMutation({
    mutationFn: (body: Schemas["ChatCanvasSave"]) => unwrap(api.POST("/chats/{id}/canvases/{canvasId}/versions", { params: { path }, body })),
    onSettled: refresh,
  });
  const restore = useMutation({
    mutationFn: (seq: number) => unwrap(api.POST("/chats/{id}/canvases/{canvasId}/restore", { params: { path }, body: { seq } })),
    onSettled: refresh,
  });
  const promote = useMutation({
    mutationFn: () => unwrap(api.POST("/chats/{id}/canvases/{canvasId}/promote", { params: { path } })),
    onSettled: refresh,
  });
  return { add, send, setStatus, remove, apply, save, restore, promote };
}
