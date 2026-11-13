"use client";

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import { useRepositoryPresets, useRepositorySources, useSaveRepositorySource } from "@/lib/api/repository-hooks";

export function RepositorySourcesSection() {
  const sources = useRepositorySources();
  const presets = useRepositoryPresets();
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
    <p className="text-14 text-fg-secondary">Save a single repository or all repositories under a GitHub username or organization. Every coding persona can find them by name. Access comes from your GitHub credential.</p>
    <div className="rounded-xl border border-border p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="type-heading">From your GitHub connection</h3>
        <button type="button" disabled={presets.isFetching} onClick={() => presets.refetch()} className="text-12 text-accent">Refresh GitHub choices</button>
      </div>
      {presets.isPending && <p className="mt-2 text-14 text-fg-secondary">Finding your GitHub account and organizations…</p>}
      {presets.error && <p role="status" className="mt-2 text-14 text-fg-secondary">{presets.error.message} You can enter a repository or owner manually below.</p>}
      {presets.data?.warning && <p role="status" className="mt-2 text-14 text-fg-secondary">{presets.data.warning}</p>}
      <ul aria-label="Connected GitHub owners" className="mt-2 flex flex-col gap-2">
        {presets.data?.owners.map((owner) => {
          const added = sources.data?.some((source) => source.kind === "organization" && source.name.toLowerCase() === owner.login.toLowerCase());
          return <li key={owner.login}>
            <button type="button" aria-label={`Add all repositories under ${owner.login}`} disabled={added || save.isPending || sources.isPending || presets.isFetching} onClick={() => save.mutate({ kind: "organization", name: owner.login })} className="flex w-full flex-wrap items-center gap-2 rounded border border-border bg-surface px-3 py-2 text-left text-14 hover:bg-hover disabled:opacity-60">
              <span className="font-medium text-fg">{owner.login}</span>
              <span className="text-fg-tertiary">{owner.organization ? "Organization" : "Your account"}</span>
              <span className="ml-auto text-accent">{added ? "Added" : "Add all repos"}</span>
            </button>
          </li>;
        })}
      </ul>
    </div>
    <details>
      <summary className="cursor-pointer text-14 text-accent">Enter a repository or owner manually</summary>
      <form className="mt-3 flex flex-wrap gap-2" onSubmit={(e) => { e.preventDefault(); save.mutate({ kind, name }, { onSuccess: () => setName("") }); }}>
        <select aria-label="Repository source type" value={kind} onChange={(e) => setKind(e.target.value as typeof kind)} className="max-w-full rounded border border-border bg-surface p-2 text-14 text-fg"><option value="repository">Single repository</option><option value="organization">All repos under a username or organization</option></select>
        <input aria-label={kind === "repository" ? "GitHub repository" : "GitHub username or organization"} value={name} onChange={(e) => setName(e.target.value)} placeholder={kind === "repository" ? "owner/repo or GitHub URL" : "e.g. liamphmurphy"} className="min-w-60 flex-1 rounded border border-border bg-surface p-2 text-14 text-fg" />
        <button type="submit" disabled={!name.trim() || save.isPending} className="rounded bg-accent px-3 py-2 text-14 text-white disabled:opacity-40">Save source</button>
      </form>
    </details>
    {(error || problem) && <p role="alert" className="text-14 text-pill-blocked-fg">{error?.message ?? problem}</p>}
    {sources.isPending && <p className="text-14 text-fg-secondary">Loading saved sources…</p>}
    <ul aria-label="Saved repository sources" className="divide-y divide-border">{sources.data?.map((source) => <li key={`${source.kind}/${source.name}`} className="flex items-center gap-3 py-2 text-14"><span className="text-fg-tertiary">{source.kind === "organization" ? "All repositories" : "Repository"}</span><span className="flex-1 truncate text-fg">{source.name}</span><button type="button" aria-label={`Remove ${source.name}`} disabled={remove.isPending} onClick={() => remove.mutate(source)} className="text-fg-secondary hover:text-fg">Remove</button></li>)}</ul>
    <button type="button" disabled={refreshing} onClick={refresh} className="self-start text-14 text-accent">{refreshing ? "Refreshing owners…" : "Refresh owner repositories"}</button>
  </div>;
}
