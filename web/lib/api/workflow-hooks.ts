"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, type Schemas } from "./client";

export type Workflow = Schemas["Workflow"];
export type WorkflowRun = Schemas["WorkflowRun"];
export type WorkflowStep = Schemas["WorkflowStep"];

const active = (runs: WorkflowRun[] | undefined) => runs?.some((r) => r.state === "running" || r.state === "waiting");

const keys = {
  all: ["workflows"] as const,
  one: (id: string) => ["workflows", id] as const,
  runs: ["workflows", "runs"] as const,
  run: (id: string) => ["workflows", "runs", id] as const,
  chat: (id: string) => ["workflows", "chat", id] as const,
};

export function useWorkflows(enabled = true) {
  return useQuery({ enabled, queryKey: keys.all, queryFn: () => unwrap(api.GET("/workflows")) });
}

/** One workflow. poll follows a persona's suggestions while a build chat is open. */
export function useWorkflow(id: string, poll = false) {
  return useQuery({ queryKey: keys.one(id), queryFn: () => unwrap(api.GET("/workflows/{id}", { params: { path: { id } } })), refetchInterval: poll ? 3000 : false });
}

export function useCreateWorkflow() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Schemas["NewWorkflow"]) => unwrap(api.POST("/workflows", { body })),
    onSettled: () => qc.invalidateQueries({ queryKey: keys.all }),
  });
}

export function useSaveWorkflow(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Schemas["WorkflowPatch"]) => unwrap(api.PATCH("/workflows/{id}", { params: { path: { id } }, body })),
    onSuccess: (w) => qc.setQueryData(keys.one(id), w),
  });
}

export function usePublishWorkflow(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => unwrap(api.POST("/workflows/{id}/publish", { params: { path: { id } } })),
    onSuccess: (w) => qc.setQueryData(keys.one(id), w),
    onSettled: () => qc.invalidateQueries({ queryKey: keys.all }),
  });
}

export function useDeleteWorkflow() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => {
      const { error, response } = await api.DELETE("/workflows/{id}", { params: { path: { id } } });
      if (error || !response.ok) throw error ?? new Error(`Delete failed (${response.status})`);
    },
    onSettled: () => qc.invalidateQueries({ queryKey: keys.all }),
  });
}

export function useResolveProposal(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { proposalId: string; action: "apply" | "discard" }) =>
      unwrap(api.POST("/workflows/{id}/proposals/{proposalId}/{action}", { params: { path: { id, proposalId: v.proposalId, action: v.action } } })),
    onSuccess: (w) => qc.setQueryData(keys.one(id), w),
  });
}

export function useWorkflowRuns() {
  return useQuery({ queryKey: keys.runs, queryFn: () => unwrap(api.GET("/workflow-runs")), refetchInterval: (q) => (active(q.state.data?.runs) ? 3000 : false) });
}

export function useWorkflowRun(id: string) {
  return useQuery({
    queryKey: keys.run(id),
    queryFn: () => unwrap(api.GET("/workflow-runs/{id}", { params: { path: { id } } })),
    refetchInterval: (q) => (q.state.data && active([q.state.data]) ? 3000 : false),
  });
}

export function useChatWorkflowRuns(chatId?: string) {
  return useQuery({
    enabled: !!chatId,
    queryKey: keys.chat(chatId ?? ""),
    queryFn: () => unwrap(api.GET("/chats/{id}/workflows", { params: { path: { id: chatId! } } })),
    refetchInterval: (q) => (active(q.state.data?.runs) ? 3000 : false),
  });
}

export function useStartWorkflow(chatId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Schemas["WorkflowRunStart"]) => unwrap(api.POST("/chats/{id}/workflows", { params: { path: { id: chatId } }, body })),
    onSettled: () => qc.invalidateQueries({ queryKey: keys.chat(chatId) }),
  });
}

export function useDecideGate(runId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Schemas["WorkflowGateDecision"]) => unwrap(api.POST("/workflow-runs/{id}/gate", { params: { path: { id: runId } }, body })),
    onSettled: () => qc.invalidateQueries({ queryKey: keys.all }),
  });
}

export function useStopWorkflowRun(runId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => unwrap(api.POST("/workflow-runs/{id}/stop", { params: { path: { id: runId } } })),
    onSettled: () => qc.invalidateQueries({ queryKey: keys.all }),
  });
}
