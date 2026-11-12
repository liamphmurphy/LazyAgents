"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, type Schemas } from "./client";
import type { paths } from "./schema";

// ---- The Designs app: the person's designs, each a design canvas in its own chat.

export type DesignSummary = Schemas["DesignSummary"];
export type DesignRef = Schemas["Design"];
export type DesignVersion = Schemas["DesignVersion"];
export type DesignStarter = NonNullable<CreateDesignBody["starter"]>;
type CreateDesignBody = NonNullable<paths["/designs"]["post"]["requestBody"]>["content"]["application/json"];

// Under ["canvases"], so a persona's turn on a design refreshes them (SSE chat.* invalidates it).
export const designKeys = {
  list: ["canvases", "designs"] as const,
  one: (id: string) => ["canvases", "designs", id] as const,
  // A version never changes, so it's cached for good.
  version: (id: string, seq: number) => ["design-versions", id, seq] as const,
};

export function useDesigns() {
  return useQuery({ queryKey: designKeys.list, queryFn: () => unwrap(api.GET("/designs")), select: (d) => d.designs });
}

/** One design: its chat, current seq, switches and pending suggestions. */
export function useDesign(id: string) {
  return useQuery({ queryKey: designKeys.one(id), queryFn: () => unwrap(api.GET("/designs/{canvasId}", { params: { path: { canvasId: id } } })) });
}

/** A version's body; seq undefined waits. */
export function useDesignVersion(id: string | undefined, seq: number | undefined) {
  return useQuery({
    queryKey: designKeys.version(id ?? "", seq ?? 0),
    queryFn: () => unwrap(api.GET("/designs/{canvasId}/versions/{seq}", { params: { path: { canvasId: id!, seq: seq! } } })),
    enabled: !!id && !!seq,
    staleTime: Infinity,
  });
}

export async function fetchDesignVersion(id: string, seq: number): Promise<DesignVersion> {
  return unwrap(api.GET("/designs/{canvasId}/versions/{seq}", { params: { path: { canvasId: id, seq } } }));
}

export async function fetchDesign(id: string): Promise<DesignRef> {
  return unwrap(api.GET("/designs/{canvasId}", { params: { path: { canvasId: id } } }));
}

/** Saves a version on base_seq; 409 (ApiError) when the design moved on. */
export function useSaveDesign(id: string) {
  return useMutation({
    mutationFn: (body: { base_seq: number; body: string; note?: string }) => unwrap(api.POST("/designs/{canvasId}/versions", { params: { path: { canvasId: id } }, body })),
  });
}

export function useCreateDesign() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: CreateDesignBody) => unwrap(api.POST("/designs", { body })),
    onSettled: () => void qc.invalidateQueries({ queryKey: designKeys.list }),
  });
}

export function useRenameDesign() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { id: string; title: string }) => unwrap(api.PATCH("/designs/{canvasId}", { params: { path: { canvasId: v.id } }, body: { title: v.title } })),
    onSettled: () => void qc.invalidateQueries({ queryKey: ["canvases"] }),
  });
}

export function useDuplicateDesign() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => unwrap(api.POST("/designs/{canvasId}/duplicate", { params: { path: { canvasId: id } } })),
    onSettled: () => void qc.invalidateQueries({ queryKey: designKeys.list }),
  });
}

export function useDeleteDesign() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => {
      const { error } = await api.DELETE("/designs/{canvasId}", { params: { path: { canvasId: id } } });
      if (error) throw new Error(error.message);
    },
    onSettled: () => void qc.invalidateQueries({ queryKey: designKeys.list }),
  });
}

/** Stores an image for a design; the caller adds it to the design's assets. */
export async function uploadDesignAsset(canvasId: string, file: Blob): Promise<Schemas["DeckAsset"]> {
  const form = new FormData();
  form.append("file", file);
  const r = await fetch(`/api/v1/designs/${canvasId}/assets`, { method: "POST", body: form, credentials: "same-origin" });
  const body = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error((body as { message?: string }).message ?? `Upload failed (${r.status})`);
  return body as Schemas["DeckAsset"];
}
