"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError, SIGN_IN_PAGE, type Schemas } from "./client";

/** A place an app dock keeps chats for: a Knowledge page, or an app's other screens. */
export type DockScope = `page:${string}` | `app:${string}`;

export const dockKeys = {
  all: ["dock-chats"] as const,
  scope: (scope: DockScope) => ["dock-chats", scope] as const,
};

async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(`/api/v1/dock-chats${path}`, {
    method,
    headers: body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (res.status === 401 && typeof window !== "undefined" && window.location.pathname !== SIGN_IN_PAGE) window.location.assign(SIGN_IN_PAGE);
  const text = await res.text();
  const data = text ? JSON.parse(text) : undefined;
  if (!res.ok) throw new ApiError(res.status, data?.code ?? "internal", data?.message ?? res.statusText, data?.field);
  return data as T;
}

/** The chats a dock keeps for one place, most recently used first. */
export function useDockChats(scope: DockScope | undefined) {
  return useQuery({
    enabled: !!scope,
    queryKey: dockKeys.scope(scope ?? "app:none"),
    queryFn: () => call<{ chats: Schemas["ChatSession"][] }>("GET", `?scope=${encodeURIComponent(scope!)}`),
  });
}

/** Keeps a chat in a dock's list, when the person starts or picks it there. */
export function useAddDockChat() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { scope: DockScope; chatId: string }) => call<void>("POST", "", { scope: v.scope, chat_id: v.chatId }),
    onSettled: (_r, _e, v) => void qc.invalidateQueries({ queryKey: dockKeys.scope(v.scope) }),
  });
}
