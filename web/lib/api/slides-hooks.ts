"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, type Schemas } from "./client";

// ---- The Slides app: the person's decks, each a deck canvas in its own chat.

export type DeckSummary = Schemas["DeckSummary"];
export type DeckRef = Schemas["MyCanvas"];

// Under ["canvases"], so a persona's turn on a deck refreshes the list (SSE chat.* invalidates it).
export const slideKeys = {
  list: ["canvases", "decks"] as const,
  one: (id: string) => ["canvases", "mine", id] as const,
};

export function useDecks() {
  return useQuery({ queryKey: slideKeys.list, queryFn: () => unwrap(api.GET("/slides")), select: (d) => d.decks });
}

/** One deck with the chat it lives in (the same endpoint Docs uses). */
export function useDeckRef(id: string) {
  return useQuery({ queryKey: slideKeys.one(id), queryFn: () => unwrap(api.GET("/canvases/{canvasId}", { params: { path: { canvasId: id } } })) });
}

export function useCreateDeck() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { title: string; theme?: "light" | "dark" | "paper" | "contrast" }) => unwrap(api.POST("/slides", { body })),
    onSettled: () => void qc.invalidateQueries({ queryKey: slideKeys.list }),
  });
}

export function useRenameDeck() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { id: string; title: string }) => unwrap(api.PATCH("/slides/{canvasId}", { params: { path: { canvasId: v.id } }, body: { title: v.title } })),
    onSettled: () => void qc.invalidateQueries({ queryKey: ["canvases"] }),
  });
}

export function useDuplicateDeck() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => unwrap(api.POST("/slides/{canvasId}/duplicate", { params: { path: { canvasId: id } } })),
    onSettled: () => void qc.invalidateQueries({ queryKey: slideKeys.list }),
  });
}

export function useDeleteDeck() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => {
      const { error } = await api.DELETE("/slides/{canvasId}", { params: { path: { canvasId: id } } });
      if (error) throw new Error(error.message);
    },
    onSettled: () => void qc.invalidateQueries({ queryKey: slideKeys.list }),
  });
}

/** Stores an image for a deck; the caller adds it to the deck's assets. */
export async function uploadDeckAsset(canvasId: string, file: Blob): Promise<Schemas["DeckAsset"]> {
  const form = new FormData();
  form.append("file", file);
  const r = await fetch(`/api/v1/slides/${canvasId}/assets`, { method: "POST", body: form, credentials: "same-origin" });
  const body = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error((body as { message?: string }).message ?? `Upload failed (${r.status})`);
  return body as Schemas["DeckAsset"];
}
