"use client";

// ---- The Library: the caller's permanent keepsakes from assistant chats.

import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api, ApiError, unwrap, type Schemas } from "./client";

export type LibraryItem = Schemas["LibraryItem"];
export type LibraryItemKind = Schemas["LibraryItemKind"];

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
    onSettled: () => qc.invalidateQueries({ queryKey: libraryKeys.all }),
  });
}
