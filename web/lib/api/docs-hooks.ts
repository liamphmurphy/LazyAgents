"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, type Schemas } from "./client";

// ---- The Docs app: the person's text canvases, each opened full page beside its chat.

export type Doc = Schemas["MyCanvas"];

// Under ["canvases"], so a turn that edits a document refreshes the list (SSE chat.* invalidates it).
const docKeys = {
  one: (id: string) => ["canvases", "mine", id] as const,
};

/** One document with the chat it lives in. */
export function useDoc(id: string) {
  return useQuery({ queryKey: docKeys.one(id), queryFn: () => unwrap(api.GET("/canvases/{canvasId}", { params: { path: { canvasId: id } } })) });
}

/** Where a document prints from; the page opens the print dialog itself. */
export const docPrintHref = (id: string) => `/print/docs/${id}`;

/** Downloads a file named after the document's title. */
export function downloadFile(title: string, ext: string, blob: Blob) {
  const name = `${title.replace(/[\\/:*?"<>|]+/g, " ").trim() || "document"}.${ext}`;
  const url = URL.createObjectURL(blob);
  const a = Object.assign(document.createElement("a"), { href: url, download: name });
  document.body.append(a);
  a.click();
  a.remove();
  // Revoking at once can cancel the download before the browser reads it.
  setTimeout(() => URL.revokeObjectURL(url), 0);
}

/** Records that the person finished reviewing a document up to a version. */
export function useMarkDocReviewed(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (seq: number) => unwrap(api.POST("/docs/{canvasId}/reviewed", { params: { path: { canvasId: id } }, body: { seq } })),
    onSuccess: (r) => qc.setQueryData<Doc>(docKeys.one(id), (d) => d && { ...d, reviewed_seq: r.reviewed_seq }),
  });
}

/** Keeps a copy of the document as it is now in the person's Library. */
export function useSaveDocToLibrary(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => unwrap(api.POST("/docs/{canvasId}/library", { params: { path: { canvasId: id } } })),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["library"] }),
  });
}

/** Publishes the document as it is now as a wiki page agents can read; `replace` overwrites a page that exists (a 409 otherwise). */
export function usePromoteDocToWiki(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (replace: boolean) => unwrap(api.POST("/docs/{canvasId}/wiki", { params: { path: { canvasId: id } }, body: { replace } })),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["wiki"] }),
  });
}
