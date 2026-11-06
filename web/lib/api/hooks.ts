"use client";

import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  api,
  ApiError,
  postForm,
  unwrap,
  type AgentDetail,
  type AgentKind,
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
  ready?: boolean;
  epic?: string;
  project?: string;
  view?: TicketView;
  status?: TicketStatus;
  assignee?: string;
  label?: string;
  q?: string;
};

// Query keys. The shell SSE subscription invalidates these prefixes.
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
    // Slow fallback while the SSE connection is unavailable.
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

// Slow fallback for live tickets if the realtime connection is interrupted.
const LIVE_POLL_MS = 5000;

export function useTicket(key: string) {
  return useQuery({
    enabled: !!key,
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
      qc.invalidateQueries({ queryKey: ["knowledge"] }); // [[links]] in the description
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
      qc.invalidateQueries({ queryKey: ["knowledge"] }); // comments can carry [[links]]
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
      qc.invalidateQueries({ queryKey: ["knowledge"] });
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
    mutationFn: (body: { handle: string; name: string; role?: string; color_index?: number; kind?: AgentKind }) =>
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
    qc.invalidateQueries({ queryKey: ["inbox"] });
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

// ----------------------------------------------------- dispatch & runs (§6)

export function useDispatchPreview(key: string | undefined) {
  return useQuery({
    queryKey: ["tickets", "dispatch", key] as const,
    queryFn: () => unwrap(api.GET("/tickets/{key}/dispatch", { params: { path: { key: key! } } })),
    enabled: !!key,
    staleTime: 5_000,
  });
}

export function useDispatch() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (keys: string[]) => {
      if (keys.length === 1) {
        await unwrap(api.POST("/tickets/{key}/dispatch", { params: { path: { key: keys[0] } } }));
        return 1;
      }
      return unwrap(api.POST("/tickets/dispatch", { body: { keys } })).then((d) => d.dispatched);
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: keys.tickets });
      qc.invalidateQueries({ queryKey: keys.bootstrap });
      qc.invalidateQueries({ queryKey: ["runner"] });
    },
  });
}

export function useRunnerStatus() {
  return useQuery({
    queryKey: ["runner"] as const,
    queryFn: () => unwrap(api.GET("/runner")),
    refetchInterval: 15_000,
  });
}

const RUN_DONE = ["completed", "failed", "cancelled"];

/**
 * A run with its turns and events. SSE invalidates this query; polling is a
 * fallback. Events are fetched incrementally with ?after=seq.
 */
function runLogOptions(qc: ReturnType<typeof useQueryClient>, id: string) {
  return {
    queryKey: ["runs", id] as const,
    queryFn: async () => {
      const prev = qc.getQueryData<Schemas["RunDetail"]>(["runs", id]);
      const after = prev?.events.at(-1)?.seq ?? 0;
      const d = await unwrap(api.GET("/runs/{id}", { params: { path: { id }, query: { after } } }));
      return prev ? { ...d, events: [...prev.events, ...d.events] } : d;
    },
    refetchInterval: (q: { state: { data?: Schemas["RunDetail"] } }) =>
      q.state.data && RUN_DONE.includes(q.state.data.run.state) ? false as const : 2_000,
  };
}

export function useRunLog(id: string) {
  const qc = useQueryClient();
  return useQuery(runLogOptions(qc, id));
}

/** Each delegated run has its own event cursor and live query. */
export function useDelegatedRunLogs(runs: Schemas["Run"][]) {
  const qc = useQueryClient();
  return useQueries({ queries: runs.map((run) => runLogOptions(qc, run.id)) });
}

export function useControlRunById(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (action: "pause" | "resume" | "cancel" | "retry") => unwrap(api.POST("/runs/{id}/{action}", { params: { path: { id, action } } })),
    onSettled: () => {
      qc.invalidateQueries({ queryKey: ["runs", id] });
      qc.invalidateQueries({ queryKey: keys.tickets });
    },
  });
}

// ------------------------------------------------------------- wiki (§12)

export const wikiKeys = {
  all: ["wiki"] as const,
  tree: ["wiki", "tree"] as const,
  page: (path: string) => ["wiki", "page", path.toLowerCase()] as const,
  suggestions: (artifactId?: string) => ["wiki", "suggestions", artifactId ?? ""] as const,
  // Links, backlinks and graphs change whenever anything is saved, so they
  // share one prefix that every save invalidates.
  knowledge: ["knowledge"] as const,
};

export function useWikiTree() {
  return useQuery({ queryKey: wikiKeys.tree, queryFn: () => unwrap(api.GET("/wiki/pages")) });
}

export function useWikiPage(path: string) {
  return useQuery({
    queryKey: wikiKeys.page(path),
    queryFn: () => unwrap(api.GET("/wiki/page", { params: { query: { path } } })),
    enabled: path !== "",
    retry: (n, e) => !(e instanceof ApiError && e.status === 404) && n < 2,
  });
}

/** Everything a page save can change: the tree, pages, links, artifacts lists. */
function useInvalidateWiki() {
  const qc = useQueryClient();
  return () => {
    qc.invalidateQueries({ queryKey: wikiKeys.all });
    qc.invalidateQueries({ queryKey: wikiKeys.knowledge });
    qc.invalidateQueries({ queryKey: keys.artifacts });
    qc.invalidateQueries({ queryKey: keys.bootstrap });
    qc.invalidateQueries({ queryKey: ["inbox"] });
  };
}

export function useCreateWikiPage() {
  const invalidate = useInvalidateWiki();
  return useMutation({
    mutationFn: (body: Schemas["NewWikiPage"]) => unwrap(api.POST("/wiki/pages", { body })),
    onSuccess: invalidate,
  });
}

export function useSaveWikiPage(id: string) {
  const invalidate = useInvalidateWiki();
  return useMutation({
    mutationFn: (body: Schemas["WikiPageSave"]) => unwrap(api.PUT("/wiki/pages/{id}", { params: { path: { id } }, body })),
    onSuccess: invalidate,
  });
}

export function useWikiSuggestions(artifactId?: string) {
  return useQuery({
    queryKey: wikiKeys.suggestions(artifactId),
    queryFn: () =>
      unwrap(api.GET("/wiki/suggestions", { params: { query: artifactId ? { artifact_id: artifactId } : {} } })).then((d) => d.suggestions),
  });
}

export function useDecideSuggestion() {
  const invalidate = useInvalidateWiki();
  return useMutation({
    mutationFn: async ({ id, decision }: { id: string; decision: "accept" | "reject" }) => {
      const { error, response } = await api.POST("/wiki/suggestions/{versionId}/{decision}", { params: { path: { versionId: id, decision } } });
      if (error) await unwrap(Promise.resolve({ error, response }));
    },
    onSuccess: invalidate,
  });
}

export function useImportWiki() {
  const invalidate = useInvalidateWiki();
  return useMutation({
    mutationFn: (file: File) => {
      const f = new FormData();
      f.append("file", file);
      return postForm<Schemas["WikiImportResult"]>("/wiki/import", f);
    },
    onSuccess: invalidate,
  });
}

/** Resolves [[link]] texts in one request; cached per set of refs. */
export function useResolvedLinks(refs: string[]) {
  const sorted = [...new Set(refs)].sort();
  return useQuery({
    queryKey: [...wikiKeys.knowledge, "resolve", sorted] as const,
    queryFn: () => unwrap(api.GET("/knowledge/resolve", { params: { query: { ref: sorted } } })).then((d) => d.links),
    enabled: sorted.length > 0,
    staleTime: 30_000,
  });
}

export function useLinkSuggest(q: string, enabled = true) {
  return useQuery({
    queryKey: [...wikiKeys.knowledge, "suggest", q] as const,
    queryFn: () => unwrap(api.GET("/knowledge/suggest", { params: { query: { q } } })).then((d) => d.targets),
    enabled,
    staleTime: 10_000,
    placeholderData: (prev) => prev,
  });
}

export function useBacklinks(kind: Schemas["NodeKind"], id: string | undefined) {
  return useQuery({
    queryKey: [...wikiKeys.knowledge, "backlinks", kind, id] as const,
    queryFn: () => unwrap(api.GET("/knowledge/backlinks", { params: { query: { kind, id: id! } } })),
    enabled: !!id,
  });
}

export function useGraph(kind: Schemas["NodeKind"], id: string | undefined, depth = 2) {
  return useQuery({
    queryKey: [...wikiKeys.knowledge, "graph", kind, id, depth] as const,
    queryFn: () => unwrap(api.GET("/knowledge/graph", { params: { query: { kind, id: id!, depth } } })),
    enabled: !!id,
  });
}

export function useLinkMention() {
  const qc = useQueryClient();
  const invalidate = useInvalidateWiki();
  return useMutation({
    mutationFn: async (body: { target_kind: Schemas["NodeKind"]; target_id: string; source_kind: "artifact" | "ticket"; source_id: string }) => {
      const { error, response } = await api.POST("/knowledge/link-mention", { body });
      if (error) await unwrap(Promise.resolve({ error, response }));
    },
    onSuccess: () => {
      invalidate();
      qc.invalidateQueries({ queryKey: keys.tickets });
    },
  });
}

export function useKnowledgeSearch(q: string) {
  return useQuery({
    queryKey: [...wikiKeys.knowledge, "search", q] as const,
    queryFn: () => unwrap(api.GET("/knowledge/search", { params: { query: { q } } })).then((d) => d.hits),
    enabled: q.trim().length > 1,
    placeholderData: (prev) => prev,
  });
}

export const PATTERN_TOPICS = ["Go", "React", "Postgres", "Testing", "API", "Architecture", "Process"] as const;

/** Fuzzy pattern finder; an empty q browses every pattern. */
export function useFindPatterns(q: string, topic?: string) {
  return useQuery({
    queryKey: [...wikiKeys.all, "patterns", q, topic ?? ""] as const,
    queryFn: () =>
      unwrap(api.GET("/wiki/patterns", { params: { query: { q: q || undefined, topic: topic || undefined, limit: 50 } } })).then((d) => d.patterns),
    placeholderData: (prev) => prev,
  });
}

// ------------------------------------------------------------- attention
export function useInbox(project?: string) {
  return useQuery({queryKey:["inbox",project??""],queryFn:()=>unwrap(api.GET("/inbox",{params:{query:{project}}})).then(d=>d.items)});
}
export function useReadInbox() {
 const qc=useQueryClient();
 return useMutation({mutationFn:async(id:string)=>{
  const r=await api.POST("/inbox/read",{body:{id}});
  if(!r.response.ok)throw new Error("Couldn't mark item read");
 },onSuccess:()=>{void qc.invalidateQueries({queryKey:["inbox"]});void qc.invalidateQueries({queryKey:keys.bootstrap});}});
}
export function useSearch(q:string,project?:string) {
 return useQuery({queryKey:["search",q,project??""],queryFn:()=>unwrap(api.GET("/search",{params:{query:{q,project}}})).then(d=>d.hits),enabled:q.trim().length>0});
}

// ------------------------------------------------------------- overviews
export type DashboardRange = "24h" | "7d" | "30d";
export function useDashboard(range: DashboardRange) {
 return useQuery({queryKey:["dashboard",range],queryFn:()=>unwrap(api.GET("/dashboard",{params:{query:{range}}})),refetchInterval:30_000});
}
export function useEpicOverviews() {
 return useQuery({queryKey:["epics"],queryFn:()=>unwrap(api.GET("/epics")).then(d=>d.epics)});
}
export function useAgentOverviews() {
 return useQuery({queryKey:["agent-overviews"],queryFn:()=>unwrap(api.GET("/agent-overviews")),refetchInterval:30_000});
}
export function useAgentRuns(handle:string,offset=0) {
 return useQuery({queryKey:["agent-overviews","runs",handle,offset],queryFn:()=>unwrap(api.GET("/agents/{handle}/runs",{params:{path:{handle},query:{offset}}}))});
}

// ---- Chats (ADR: Project chat sessions). SSE `chat.updated` invalidates ["chats"].

export const chatKeys = {
  all: ["chats"] as const,
  list: (project: string, archived = false) => ["chats", "list", project, archived] as const,
  detail: (id: string) => ["chats", "detail", id] as const,
  messages: (id: string) => ["chats", "messages", id] as const,
  models: ["chats", "models"] as const,
};

// Slow fallback while a turn runs and the realtime connection is unavailable.
const CHAT_POLL_MS = 3000;

export function useChats(project: string, archived = false) {
  return useQuery({
    enabled: !!project,
    queryKey: chatKeys.list(project, archived),
    queryFn: () => unwrap(api.GET("/chats", { params: { query: { project, archived } } })),
  });
}

export function useChat(id?: string) {
  return useQuery({
    enabled: !!id,
    queryKey: chatKeys.detail(id ?? ""),
    queryFn: () => unwrap(api.GET("/chats/{id}", { params: { path: { id: id! } } })),
    refetchInterval: (q) => (q.state.data?.state === "running" ? CHAT_POLL_MS : false),
  });
}

export function useChatMessages(id?: string) {
  return useQuery({
    enabled: !!id,
    queryKey: chatKeys.messages(id ?? ""),
    queryFn: () => unwrap(api.GET("/chats/{id}/messages", { params: { path: { id: id! }, query: { limit: 200 } } })),
  });
}

export function useChatModels() {
  return useQuery({
    queryKey: chatKeys.models,
    queryFn: () => unwrap(api.GET("/chat-models")),
    staleTime: 60_000,
  });
}

function useChatInvalidate() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: chatKeys.all });
}

export function useCreateChat() {
  const done = useChatInvalidate();
  return useMutation({
    mutationFn: (body: Schemas["NewChat"]) => unwrap(api.POST("/chats", { body })),
    onSettled: done,
  });
}

export function useUpdateChat() {
  const done = useChatInvalidate();
  return useMutation({
    mutationFn: (v: { id: string; patch: Schemas["ChatPatch"] }) =>
      unwrap(api.PATCH("/chats/{id}", { params: { path: { id: v.id } }, body: v.patch })),
    onSettled: done,
  });
}

export function useSendChatMessage(id: string) {
  const done = useChatInvalidate();
  return useMutation({
    mutationFn: (body: Schemas["NewChatMessage"]) =>
      unwrap(api.POST("/chats/{id}/messages", { params: { path: { id } }, body })),
    onSettled: done,
  });
}

export function useCancelChat(id: string) {
  const done = useChatInvalidate();
  return useMutation({
    mutationFn: () => unwrap(api.POST("/chats/{id}/cancel", { params: { path: { id } } })),
    onSettled: done,
  });
}

/** The current turn's tool calls/results, read from the segment run's events. */
export function useChatRunEvents(runId?: string, enabled = true) {
  return useQuery({
    enabled: !!runId && enabled,
    queryKey: ["runs", "chat", runId ?? ""] as const,
    queryFn: () => unwrap(api.GET("/runs/{id}", { params: { path: { id: runId! } } })).then((d) => d.events),
    refetchInterval: 2_000,
  });
}
