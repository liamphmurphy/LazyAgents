"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, type Schemas } from "./client";

// ---- The Docs app: the person's text canvases, each opened full page beside its chat.

export type Doc = Schemas["MyCanvas"];

// Under ["canvases"], so a turn that edits a document refreshes the list (SSE chat.* invalidates it).
const docKeys = {
  list: ["canvases", "mine"] as const,
  one: (id: string) => ["canvases", "mine", id] as const,
};

/** Every document: text canvases from the Docs app and from any chat, most recently changed first. */
export function useDocs() {
  return useQuery({
    queryKey: docKeys.list,
    queryFn: () => unwrap(api.GET("/canvases")),
    select: (d) => d.canvases.filter((c) => c.kind === "text"),
  });
}

/** One document with the chat it lives in. */
export function useDoc(id: string) {
  return useQuery({ queryKey: docKeys.one(id), queryFn: () => unwrap(api.GET("/canvases/{canvasId}", { params: { path: { canvasId: id } } })) });
}

export function useCreateDoc() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (title: string) => unwrap(api.POST("/docs", { body: { title } })),
    onSettled: () => void qc.invalidateQueries({ queryKey: docKeys.list }),
  });
}

/** Where a document prints from; the page opens the print dialog itself. */
export const docPrintHref = (id: string) => `/print/docs/${id}`;

/** Saves the document's Markdown as a file named after its title. */
export function downloadMarkdown(title: string, body: string) {
  const name = `${title.replace(/[\\/:*?"<>|]+/g, " ").trim() || "document"}.md`;
  const url = URL.createObjectURL(new Blob([body], { type: "text/markdown;charset=utf-8" }));
  const a = Object.assign(document.createElement("a"), { href: url, download: name });
  document.body.append(a);
  a.click();
  a.remove();
  // Revoking at once can cancel the download before the browser reads it.
  setTimeout(() => URL.revokeObjectURL(url), 0);
}
