"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  api,
  postForm,
  unwrap,
  type Artifact,
  type ArtifactType,
  type Schemas,
  type Ticket,
  type TicketPatch,
  type TicketStatus,
} from "./client";

type NewTicket = Schemas["NewTicket"];

export type TicketView = "active" | "backlog" | "agent_owned" | "needs_human" | "all";
export type TicketFilters = {
  project?: string;
  view?: TicketView;
  status?: TicketStatus;
  assignee?: string;
  label?: string;
  q?: string;
};

// Query keys. SSE invalidations (Phase 6) target these prefixes.
export const keys = {
  bootstrap: ["bootstrap"] as const,
  tickets: ["tickets"] as const,
  ticketList: (f: TicketFilters) => ["tickets", "list", f] as const,
  ticket: (key: string) => ["tickets", "detail", key] as const,
  activity: (key: string) => ["tickets", "activity", key] as const,
  artifacts: ["artifacts"] as const,
  artifactList: (f: ArtifactFilters) => ["artifacts", "list", f] as const,
  artifact: (id: string) => ["artifacts", "detail", id] as const,
};

export type ArtifactFilters = { type?: ArtifactType; project?: string; q?: string };

export function useBootstrap() {
  return useQuery({
    queryKey: keys.bootstrap,
    queryFn: () => unwrap(api.GET("/bootstrap")),
    staleTime: 60_000,
  });
}

export function useTickets(f: TicketFilters) {
  return useQuery({
    queryKey: keys.ticketList(f),
    queryFn: () => unwrap(api.GET("/tickets", { params: { query: f } })).then((d) => d.tickets),
  });
}

export function useTicket(key: string) {
  return useQuery({ queryKey: keys.ticket(key), queryFn: () => unwrap(api.GET("/tickets/{key}", { params: { path: { key } } })) });
}

export function useActivity(key: string) {
  return useQuery({
    queryKey: keys.activity(key),
    queryFn: () => unwrap(api.GET("/tickets/{key}/activity", { params: { path: { key } } })).then((d) => d.events),
  });
}

/** Patch a ticket. The detail cache updates optimistically; lists refetch. */
export function useUpdateTicket(key: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (patch: TicketPatch) =>
      unwrap(api.PATCH("/tickets/{key}", { params: { path: { key } }, body: patch })),
    onMutate: async (patch) => {
      await qc.cancelQueries({ queryKey: keys.ticket(key) });
      const prev = qc.getQueryData<Ticket>(keys.ticket(key));
      if (prev) {
        const next = { ...prev };
        if (patch.title !== undefined) next.title = patch.title;
        if (patch.description_md !== undefined) next.description_md = patch.description_md;
        if (patch.status !== undefined) next.status = patch.status;
        if (patch.priority !== undefined) next.priority = patch.priority;
        qc.setQueryData(keys.ticket(key), next);
      }
      return { prev };
    },
    onError: (_e, _p, ctx) => ctx?.prev && qc.setQueryData(keys.ticket(key), ctx.prev),
    onSuccess: (t) => qc.setQueryData(keys.ticket(key), t),
    onSettled: () => {
      qc.invalidateQueries({ queryKey: ["tickets", "list"] });
      qc.invalidateQueries({ queryKey: keys.activity(key) });
    },
  });
}

export function useBulkUpdate() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { keys: string[]; patch: TicketPatch; attach_artifact_id?: string }) =>
      unwrap(api.POST("/tickets/bulk", { body })),
    onSettled: () => {
      qc.invalidateQueries({ queryKey: keys.tickets });
      qc.invalidateQueries({ queryKey: keys.artifacts });
    },
  });
}

export function useCreateTicket() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: NewTicket) => unwrap(api.POST("/tickets", { body })),
    onSuccess: (t) => {
      qc.setQueryData(keys.ticket(t.key), t);
      qc.invalidateQueries({ queryKey: ["tickets", "list"] });
    },
  });
}

function useTicketChildMutation<V>(key: string, fn: (v: V) => Promise<unknown>) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSettled: () => {
      qc.invalidateQueries({ queryKey: keys.ticket(key) });
      qc.invalidateQueries({ queryKey: keys.activity(key) });
      qc.invalidateQueries({ queryKey: ["tickets", "list"] });
    },
  });
}

export function useAddCriterion(key: string) {
  return useTicketChildMutation(key, (text: string) =>
    unwrap(api.POST("/tickets/{key}/criteria", { params: { path: { key } }, body: { text } })),
  );
}

export function useUpdateCriterion(key: string) {
  const qc = useQueryClient();
  const m = useTicketChildMutation(key, (v: { id: string; done?: boolean; text?: string }) =>
    unwrap(api.PATCH("/criteria/{id}", { params: { path: { id: v.id } }, body: { done: v.done, text: v.text } })),
  );
  return {
    ...m,
    mutate: (v: { id: string; done?: boolean; text?: string }) => {
      // Tick instantly; the server response reconciles.
      const prev = qc.getQueryData<Ticket>(keys.ticket(key));
      if (prev) {
        qc.setQueryData(keys.ticket(key), {
          ...prev,
          criteria: prev.criteria.map((c) => (c.id === v.id ? { ...c, ...(v.done !== undefined && { done: v.done }), ...(v.text !== undefined && { text: v.text }) } : c)),
        });
      }
      m.mutate(v);
    },
  };
}

export function useDeleteCriterion(key: string) {
  return useTicketChildMutation(key, (id: string) =>
    api.DELETE("/criteria/{id}", { params: { path: { id } } }).then((r) => {
      if (!r.response.ok) throw new Error("delete failed");
    }),
  );
}

export function useComment(key: string) {
  return useTicketChildMutation(key, (v: { body_md: string; parent_comment_id?: string }) =>
    unwrap(api.POST("/tickets/{key}/comments", { params: { path: { key } }, body: v })),
  );
}

// ---------------------------------------------------------------- artifacts

export function useArtifacts(f: ArtifactFilters) {
  return useQuery({
    queryKey: keys.artifactList(f),
    queryFn: () => unwrap(api.GET("/artifacts", { params: { query: f } })),
  });
}

export function useArtifact(id: string | undefined) {
  return useQuery({
    queryKey: keys.artifact(id ?? ""),
    queryFn: () => unwrap(api.GET("/artifacts/{id}", { params: { path: { id: id! } } })),
    enabled: !!id,
  });
}

/** Create from FormData (project_key, type, title?, body_md | file, …). */
export function useCreateArtifact() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (form: FormData) => postForm<Artifact>("/artifacts", form),
    onSuccess: (a) => {
      qc.setQueryData(keys.artifact(a.id), a);
      qc.invalidateQueries({ queryKey: ["artifacts", "list"] });
    },
  });
}

export function useNewVersion(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (form: FormData) => postForm<Artifact>(`/artifacts/${id}/versions`, form),
    onSuccess: (a) => {
      qc.setQueryData(keys.artifact(a.id), a);
      qc.invalidateQueries({ queryKey: ["artifacts", "list"] });
    },
  });
}

export function useUpdateArtifact(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { title?: string; summary?: string; epic_id?: string | null }) =>
      unwrap(api.PATCH("/artifacts/{id}", { params: { path: { id } }, body })),
    onSuccess: (a) => {
      qc.setQueryData(keys.artifact(a.id), a);
      qc.invalidateQueries({ queryKey: ["artifacts", "list"] });
    },
  });
}

function useTicketArtifactMutation<V>(key: string, fn: (v: V) => Promise<Ticket>) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: (t) => qc.setQueryData(keys.ticket(key), t),
    onSettled: () => {
      qc.invalidateQueries({ queryKey: keys.activity(key) });
      qc.invalidateQueries({ queryKey: keys.artifacts });
    },
  });
}

export function useAttachArtifact(key: string) {
  return useTicketArtifactMutation(key, (artifactId: string) =>
    unwrap(api.POST("/tickets/{key}/artifacts", { params: { path: { key } }, body: { artifact_id: artifactId } })),
  );
}

export function useDetachArtifact(key: string) {
  return useTicketArtifactMutation(key, (artifactId: string) =>
    unwrap(api.DELETE("/tickets/{key}/artifacts/{artifactId}", { params: { path: { key, artifactId } } })),
  );
}
