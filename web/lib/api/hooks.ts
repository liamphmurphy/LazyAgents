"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  api,
  postForm,
  unwrap,
  type AgentDetail,
  type AgentOverride,
  type AgentSpec,
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
  agent: (handle: string) => ["agents", handle] as const,
  resolved: (handle: string, project?: string) => ["agents", handle, "resolved", project ?? ""] as const,
};

export type ArtifactFilters = { type?: ArtifactType; project?: string; q?: string };

export function useBootstrap() {
  return useQuery({
    queryKey: keys.bootstrap,
    queryFn: () => unwrap(api.GET("/bootstrap")),
    staleTime: 60_000,
    // Keeps the inbox badge and live-agent count fresh until SSE (Phase 6).
    refetchInterval: 20_000,
  });
}

export function useTickets(f: TicketFilters) {
  return useQuery({
    queryKey: keys.ticketList(f),
    queryFn: () => unwrap(api.GET("/tickets", { params: { query: f } })).then((d) => d.tickets),
  });
}

const LIVE_RUN = ["queued", "starting", "running", "waiting", "paused", "blocked"];

/** True while the ticket has a run an agent may still be working on. */
export function hasLiveRun(t?: Ticket) {
  return !!t?.run && LIVE_RUN.includes(t.run.state);
}

// Until SSE lands (Phase 6), tickets with a live run poll so agent progress
// and approvals show up without a reload.
const LIVE_POLL_MS = 5000;

export function useTicket(key: string) {
  return useQuery({
    queryKey: keys.ticket(key),
    queryFn: () => unwrap(api.GET("/tickets/{key}", { params: { path: { key } } })),
    refetchInterval: (q) => (hasLiveRun(q.state.data) ? LIVE_POLL_MS : false),
  });
}

export function useActivity(key: string, live = false) {
  return useQuery({
    queryKey: keys.activity(key),
    queryFn: () => unwrap(api.GET("/tickets/{key}/activity", { params: { path: { key } } })).then((d) => d.events),
    refetchInterval: live ? LIVE_POLL_MS : false,
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

// ------------------------------------------------------------------- agents

export function useAgent(handle: string) {
  return useQuery({ queryKey: keys.agent(handle), queryFn: () => unwrap(api.GET("/agents/{handle}", { params: { path: { handle } } })) });
}

export function useResolved(handle: string, project?: string) {
  return useQuery({
    queryKey: keys.resolved(handle, project),
    queryFn: () => unwrap(api.GET("/agents/{handle}/resolved", { params: { path: { handle }, query: { project } } })),
  });
}

function useAgentMutation<V>(handle: string, fn: (v: V) => Promise<AgentDetail>) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: (d) => {
      qc.setQueryData(keys.agent(handle), d);
      qc.invalidateQueries({ queryKey: ["agents", handle, "resolved"] });
      qc.invalidateQueries({ queryKey: keys.bootstrap });
    },
  });
}

export type DefinitionInput = { name: string; role: string; specialty: string; color_index: number; spec: AgentSpec; note?: string };

export function useSaveDefinition(handle: string) {
  return useAgentMutation(handle, (body: DefinitionInput) =>
    unwrap(api.PUT("/agents/{handle}/definition", { params: { path: { handle } }, body })),
  );
}

export function useSetOverride(handle: string) {
  return useAgentMutation(handle, (v: { project: string; override: AgentOverride }) =>
    unwrap(api.PUT("/agents/{handle}/overrides/{project}", { params: { path: { handle, project: v.project } }, body: v.override })),
  );
}

export function useCreateAgent() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { handle: string; name: string; role?: string; color_index?: number }) =>
      unwrap(api.POST("/agents", { body })),
    onSuccess: (d) => {
      qc.setQueryData(keys.agent(d.agent.handle), d);
      qc.invalidateQueries({ queryKey: keys.bootstrap });
    },
  });
}

// ------------------------------------------------------- tokens and secrets

export function useAgentTokens(handle: string) {
  return useQuery({
    queryKey: ["agents", handle, "tokens"],
    queryFn: () => unwrap(api.GET("/agents/{handle}/tokens", { params: { path: { handle } } })),
  });
}

export function useCreateToken(handle: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (label: string) => unwrap(api.POST("/agents/{handle}/tokens", { params: { path: { handle } }, body: { label } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["agents", handle, "tokens"] }),
  });
}

export function useRevokeToken(handle: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (tokenId: string) => {
      const r = await api.DELETE("/agents/{handle}/tokens/{tokenId}", { params: { path: { handle, tokenId } } });
      if (!r.response.ok) throw new Error("Couldn't revoke the token");
    },
    onSettled: () => qc.invalidateQueries({ queryKey: ["agents", handle, "tokens"] }),
  });
}

export function useSecrets() {
  return useQuery({ queryKey: ["secrets"], queryFn: () => unwrap(api.GET("/secrets")) });
}

export function useSetSecret() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { name: string; value: string; grants_scope?: string; project_key?: string }) => unwrap(api.PUT("/secrets", { body })),
    onSettled: () => qc.invalidateQueries({ queryKey: ["secrets"] }),
  });
}

export function useDeleteSecret() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => {
      const r = await api.DELETE("/secrets/{id}", { params: { path: { id } } });
      if (!r.response.ok) throw new Error("Couldn't delete the secret");
    },
    onSettled: () => qc.invalidateQueries({ queryKey: ["secrets"] }),
  });
}

// --------------------------------------------------------- runs, approvals

function useTicketRefresh(key: string) {
  const qc = useQueryClient();
  return () => {
    qc.invalidateQueries({ queryKey: keys.ticket(key) });
    qc.invalidateQueries({ queryKey: keys.activity(key) });
    qc.invalidateQueries({ queryKey: keys.bootstrap });
  };
}

export function useDecideApproval(key: string) {
  const refresh = useTicketRefresh(key);
  return useMutation({
    mutationFn: (v: { id: string; decision: "approve" | "deny"; note?: string }) =>
      unwrap(api.POST("/approvals/{id}/{decision}", { params: { path: { id: v.id, decision: v.decision } }, body: { note: v.note } })),
    onSettled: refresh,
  });
}

export function useControlRun(key: string) {
  const refresh = useTicketRefresh(key);
  return useMutation({
    mutationFn: (v: { id: string; action: "pause" | "resume" | "cancel" }) =>
      unwrap(api.POST("/runs/{id}/{action}", { params: { path: { id: v.id, action: v.action } } })),
    onSettled: refresh,
  });
}
