"use client";

// ---- The Library: the caller's permanent keepsakes from assistant chats.

import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api, ApiError, postForm, unwrap, type Schemas } from "./client";

export type LibraryItem = Schemas["LibraryItem"];
export type LibraryItemKind = Schemas["LibraryItemKind"];
export type LibraryReferenceRole = Schemas["LibraryReferenceRole"];

export const libraryKeys = {
  all: ["library"] as const,
  list: (kind?: LibraryItemKind) => ["library", "list", kind ?? ""] as const,
};

/** Newest first, a page at a time. */
export function useLibrary(kind?: LibraryItemKind) {
  return useInfiniteQuery({
    queryKey: libraryKeys.list(kind),
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      unwrap(api.GET("/library", { params: { query: { ...(kind ? { kind } : {}), ...(pageParam ? { offset: pageParam } : {}) } } })),
    getNextPageParam: (last, pages) => (last.has_more ? pages.reduce((n, p) => n + p.items.length, 0) : undefined),
  });
}

/** Already gone (404) counts as removed. */
export function useDeleteLibraryItem() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (itemId: string) => {
      const { error, response } = await api.DELETE("/library/{itemId}", { params: { path: { itemId } } });
      if (response.ok || response.status === 404) return;
      const e = (error ?? {}) as Partial<Schemas["Error"]>;
      throw new ApiError(response.status, e.code ?? "internal", e.message ?? response.statusText, e.field);
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: libraryKeys.all });
      // A card shows Saved until its link is removed.
      qc.invalidateQueries({ queryKey: ["link-previews"] });
    },
  });
}

/** Multipart POST of one image of the person's own (PNG, JPEG, GIF or WebP, up to 25 MB). */
export function useUploadLibraryItem() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (file: File) => {
      const form = new FormData();
      form.append("file", file);
      return postForm<LibraryItem>("/library", form);
    },
    onSettled: () => qc.invalidateQueries({ queryKey: libraryKeys.all }),
  });
}

/** Keeps an SVG from an assistant reply. Saving the same one again changes nothing. */
export function useSaveLibraryDiagram() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Schemas["LibraryDiagramSave"]) => unwrap(api.POST("/library/diagrams", { body })),
    onSettled: () => qc.invalidateQueries({ queryKey: libraryKeys.all }),
  });
}

/** Keeps a link a reply recommends. Saving the same one again changes nothing. */
export function useSaveLibraryLink() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Schemas["LibraryLinkSave"]) => unwrap(api.POST("/library/links", { body })),
    onSettled: () => {
      qc.invalidateQueries({ queryKey: libraryKeys.all });
      qc.invalidateQueries({ queryKey: ["link-previews"] });
    },
  });
}

/** Makes the item the person's (role self) or a persona's reference image, replacing the previous one. */
export function useSetLibraryReference() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { itemId: string; role: LibraryReferenceRole; personaId?: string }) => {
      const body = v.role === "persona" ? { role: v.role, persona_id: v.personaId } : { role: v.role };
      const { error, response } = await api.PUT("/library/{itemId}/reference", { params: { path: { itemId: v.itemId } }, body });
      if (response.ok) return;
      const e = (error ?? {}) as Partial<Schemas["Error"]>;
      throw new ApiError(response.status, e.code ?? "internal", e.message ?? response.statusText, e.field);
    },
    onSettled: () => qc.invalidateQueries({ queryKey: libraryKeys.all }),
  });
}

/** The image stays in the Library; it just stops being a reference. */
export function useClearLibraryReference() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (itemId: string) => {
      const { error, response } = await api.DELETE("/library/{itemId}/reference", { params: { path: { itemId } } });
      if (response.ok) return;
      const e = (error ?? {}) as Partial<Schemas["Error"]>;
      throw new ApiError(response.status, e.code ?? "internal", e.message ?? response.statusText, e.field);
    },
    onSettled: () => qc.invalidateQueries({ queryKey: libraryKeys.all }),
  });
}
