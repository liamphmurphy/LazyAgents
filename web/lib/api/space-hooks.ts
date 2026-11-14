"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, postForm, unwrap, type Schemas } from "./client";
import { chatKeys, memoryKeys } from "./hooks";
import { spaceHex } from "../spaces";

export type Space = Schemas["ChatSpace"];
export type SpaceSource = Schemas["ChatSpaceSource"];

// ---- Spaces: groups of chats with shared instructions, sources and memory.

export const spaceKeys = {
  all: ["spaces"] as const,
  list: ["spaces", "list"] as const,
  detail: (id: string) => ["spaces", "detail", id] as const,
  sources: (id: string) => ["spaces", "sources", id] as const,
  canvases: ["spaces", "canvases"] as const,
};

export function useSpaces() {
  return useQuery({ queryKey: spaceKeys.list, queryFn: () => unwrap(api.GET("/chat-spaces")) });
}

export function useSpace(id?: string) {
  return useQuery({
    enabled: !!id,
    queryKey: spaceKeys.detail(id ?? ""),
    queryFn: () => unwrap(api.GET("/chat-spaces/{spaceId}", { params: { path: { spaceId: id! } } })),
  });
}

/** A space's chats with every persona, newest activity first, each with a preview of its last reply. */
export function useSpaceChats(id?: string, archived = false) {
  return useQuery({
    enabled: !!id,
    queryKey: chatKeys.list(archived, "all", id ?? ""),
    queryFn: () => unwrap(api.GET("/chats", { params: { query: { archived, persona_id: "all", space: id! } } })),
  });
}

function useSpaceInvalidate() {
  const qc = useQueryClient();
  // Chats carry their space's name and counts; memories their space's name; a space's work
  // (its key, name and color) is in the bootstrap's projects and every ticket view.
  return () =>
    Promise.all([
      qc.invalidateQueries({ queryKey: spaceKeys.all }),
      qc.invalidateQueries({ queryKey: chatKeys.all }),
      qc.invalidateQueries({ queryKey: memoryKeys.all }),
      qc.invalidateQueries({ queryKey: ["bootstrap"] }),
      qc.invalidateQueries({ queryKey: ["projects"] }),
    ]);
}

export function useCreateSpace() {
  const done = useSpaceInvalidate();
  return useMutation({
    mutationFn: (body: Schemas["NewChatSpace"]) => unwrap(api.POST("/chat-spaces", { body })),
    onSettled: () => void done(),
  });
}

export function useUpdateSpace() {
  const done = useSpaceInvalidate();
  return useMutation({
    mutationFn: (v: { id: string; patch: Schemas["ChatSpacePatch"] }) =>
      unwrap(api.PATCH("/chat-spaces/{spaceId}", { params: { path: { spaceId: v.id } }, body: v.patch })),
    onSettled: () => void done(),
  });
}

export function useDeleteSpace() {
  const qc = useQueryClient();
  const done = useSpaceInvalidate();
  return useMutation({
    mutationFn: async (id: string) => {
      const { error, response } = await api.DELETE("/chat-spaces/{spaceId}", { params: { path: { spaceId: id } } });
      if (error || !response.ok) throw error ?? new Error(`Delete failed (${response.status})`);
    },
    onSuccess: (_, id) => qc.removeQueries({ queryKey: spaceKeys.detail(id) }),
    onSettled: () => void done(),
  });
}

/** Makes a space track work: a key, and optionally its repository (LAZYAGENTS-213). */
export function useTrackWork() {
  const done = useSpaceInvalidate();
  return useMutation({
    mutationFn: (v: { id: string; work: Schemas["SpaceWorkInput"] }) =>
      unwrap(api.PUT("/chat-spaces/{spaceId}/work", { params: { path: { spaceId: v.id } }, body: v.work })),
    onSettled: () => void done(),
  });
}

/** Stops tracking work in a space; refused while its work has tickets. */
export function useStopWork() {
  const done = useSpaceInvalidate();
  return useMutation({
    mutationFn: (id: string) => unwrap(api.DELETE("/chat-spaces/{spaceId}/work", { params: { path: { spaceId: id } } })),
    onSettled: () => void done(),
  });
}

/** Changes a space's work: its repository, branch, or archiving. Its name and color are the space's. */
export function useUpdateWork() {
  const done = useSpaceInvalidate();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { space: Space; patch: { repo_url?: string; default_branch?: string; archived?: boolean } }) => {
      const w = v.space.work!;
      return unwrap(
        api.PUT("/projects/{key}", {
          params: { path: { key: w.key } },
          body: {
            name: v.space.name,
            color: spaceHex(v.space.color),
            repo_url: v.patch.repo_url ?? w.repo_url ?? "",
            default_branch: v.patch.default_branch ?? w.default_branch,
            archived: v.patch.archived ?? w.archived,
            revision: w.revision,
          },
        }),
      );
    },
    onSettled: () => Promise.all([done(), qc.invalidateQueries({ queryKey: ["tickets"] })]),
  });
}

export function useSpaceSources(id?: string) {
  return useQuery({
    enabled: !!id,
    queryKey: spaceKeys.sources(id ?? ""),
    queryFn: () => unwrap(api.GET("/chat-spaces/{spaceId}/sources", { params: { path: { spaceId: id! } } })),
  });
}

function useSourcesInvalidate() {
  const qc = useQueryClient();
  return () => Promise.all([qc.invalidateQueries({ queryKey: spaceKeys.all }), qc.invalidateQueries({ queryKey: chatKeys.all })]);
}

export function useAddSpaceSource(id: string) {
  const done = useSourcesInvalidate();
  return useMutation({
    mutationFn: (body: Schemas["NewChatSpaceSource"]) => unwrap(api.POST("/chat-spaces/{spaceId}/sources", { params: { path: { spaceId: id } }, body })),
    onSettled: () => void done(),
  });
}

export function useUploadSpaceSource(id: string) {
  const done = useSourcesInvalidate();
  return useMutation({
    mutationFn: (file: File) => {
      const form = new FormData();
      form.append("file", file);
      return postForm<SpaceSource>(`/chat-spaces/${id}/sources/upload`, form);
    },
    onSettled: () => void done(),
  });
}

export function useDeleteSpaceSource(id: string) {
  const done = useSourcesInvalidate();
  return useMutation({
    mutationFn: async (sourceId: string) => {
      const { error, response } = await api.DELETE("/chat-spaces/{spaceId}/sources/{sourceId}", { params: { path: { spaceId: id, sourceId } } });
      if (error || !response.ok) throw error ?? new Error(`Remove failed (${response.status})`);
    },
    onSettled: () => void done(),
  });
}

/** The person's canvases across their chats, for adding one as a source. */
export function useMyCanvases(enabled = true) {
  return useQuery({ enabled, queryKey: spaceKeys.canvases, queryFn: () => unwrap(api.GET("/canvases")) });
}
