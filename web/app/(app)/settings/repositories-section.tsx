"use client";

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import { useRepositorySources, useSaveRepositorySource } from "@/lib/api/repository-hooks";

export function RepositorySourcesSection() {
  const sources = useRepositorySources();
  const save = useSaveRepositorySource();
  const remove = useSaveRepositorySource(true);
  const qc = useQueryClient();
  const [kind, setKind] = useState<"repository" | "organization">("repository");
  const [name, setName] = useState("");
  const [refreshing, setRefreshing] = useState(false);
  const [problem, setProblem] = useState<string>();
  async function refresh() {
    setRefreshing(true); setProblem(undefined);
    try { await unwrap(api.GET("/repositories", { params: { query: { refresh: true } } })); await qc.invalidateQueries({ queryKey: ["repositories"] }); }
    catch (e) { setProblem(e instanceof Error ? e.message : "Could not refresh repositories"); }
    finally { setRefreshing(false); }
  }
  const error = save.error ?? remove.error ?? sources.error;
  return <div className="flex flex-col gap-3">
    <p className="text-sm text-fg-secondary">Save a single repository or all repositories under a GitHub username or organization. Every coding persona can find them by name. Access comes from your GitHub credential.</p>
    <form className="flex flex-wrap gap-2" onSubmit={(e) => { e.preventDefault(); save.mutate({ kind, name }, { onSuccess: () => setName("") }); }}>
      <select aria-label="Repository source type" value={kind} onChange={(e) => setKind(e.target.value as typeof kind)} className="max-w-full rounded border border-border bg-surface p-2 text-sm text-fg"><option value="repository">Single repository</option><option value="organization">All repos under a username or organization</option></select>
      <input aria-label={kind === "repository" ? "GitHub repository" : "GitHub username or organization"} value={name} onChange={(e) => setName(e.target.value)} placeholder={kind === "repository" ? "owner/repo or GitHub URL" : "e.g. liamphmurphy"} className="min-w-60 flex-1 rounded border border-border bg-surface p-2 text-sm text-fg" />
      <button type="submit" disabled={!name.trim() || save.isPending} className="rounded bg-accent px-3 py-2 text-sm text-white disabled:opacity-40">Save source</button>
    </form>
    {(error || problem) && <p role="alert" className="text-sm text-pill-blocked-fg">{error?.message ?? problem}</p>}
    {sources.isPending && <p className="text-sm text-fg-secondary">Loading saved sources…</p>}
    <ul className="divide-y divide-border">{sources.data?.map((source) => <li key={`${source.kind}/${source.name}`} className="flex items-center gap-3 py-2 text-sm"><span className="text-fg-tertiary">{source.kind === "organization" ? "All repositories" : "Repository"}</span><span className="flex-1 truncate text-fg">{source.name}</span><button type="button" aria-label={`Remove ${source.name}`} disabled={remove.isPending} onClick={() => remove.mutate(source)} className="text-fg-secondary hover:text-fg">Remove</button></li>)}</ul>
    <button type="button" disabled={refreshing} onClick={refresh} className="self-start text-sm text-accent">{refreshing ? "Refreshing owners…" : "Refresh owner repositories"}</button>
  </div>;
}
