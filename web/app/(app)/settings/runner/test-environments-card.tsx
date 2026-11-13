"use client";

import Link from "next/link";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import type { components } from "@/lib/api/schema";
import { absoluteTime } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { Banner, Card, Skeleton } from "@/components/ui/primitives";
import { SmallPill } from "@/components/domain/labels";

type TestEnvironment = components["schemas"]["TestEnvironment"];

const queryKey = ["runner", "test-environments"];

// What each repository's tests need in a turn. The runner infers it from CI
// and compose files and adds the Nix tools agents install; a person can
// replace it (as JSON) or forget it so the next run infers it again.
export function TestEnvironmentsCard() {
  const query = useQuery({
    queryKey,
    queryFn: () => unwrap(api.GET("/runner/test-environments")),
    retry: 1,
  });
  return <Card title="Test environments">
    <p className="mb-4 text-12 text-fg-tertiary">
      Services started next to every turn and tools installed when a run starts, per repository. The first run reads them from the
      repository’s CI workflows and compose files; tools agents install with <code>nix profile install</code> are added after each turn.
      Services declared on a project’s agent override win over these.
    </p>
    {query.isPending && <Skeleton className="h-24" />}
    {query.isError && <Banner tone="blocked" title="Couldn’t load test environments" actions={<Button variant="outline" onClick={() => void query.refetch()}>Try again</Button>}>
      {query.error instanceof Error ? query.error.message : "The API is unavailable."}
    </Banner>}
    {query.data && !query.data.environments.length && <p className="text-14 text-fg-tertiary">None yet. A run on a repository records one when its CI declares services or an agent installs a tool.</p>}
    <div className="space-y-5">
      {query.data?.environments.map((env) => <TestEnvironmentRow key={env.repo} env={env} />)}
    </div>
  </Card>;
}

function provenance(env: TestEnvironment) {
  if (env.edited_at) return `Edited ${absoluteTime(env.edited_at)}`;
  if (env.learned_from_run_id) return `Updated ${absoluteTime(env.updated_at)}`;
  return `Inferred ${absoluteTime(env.created_at)}`;
}

function TestEnvironmentRow({ env }: { env: TestEnvironment }) {
  const client = useQueryClient();
  const [draft, setDraft] = useState<string | null>(null);
  const services = env.recipe.services ?? [];
  const packages = env.recipe.nix_packages ?? [];
  const save = useMutation({
    mutationFn: (text: string) => unwrap(api.PUT("/runner/test-environments", { body: { repo: env.repo, recipe: JSON.parse(text) } })),
    onSuccess: () => { setDraft(null); void client.invalidateQueries({ queryKey }); },
  });
  const forget = useMutation({
    // 204 has no body for unwrap to return.
    mutationFn: async () => {
      const { error } = await api.DELETE("/runner/test-environments", { params: { query: { repo: env.repo } } });
      if (error) throw new Error(error.message);
    },
    onSuccess: () => void client.invalidateQueries({ queryKey }),
  });
  let parseError = "";
  if (draft !== null) {
    try { JSON.parse(draft); } catch (e) { parseError = e instanceof Error ? e.message : "Invalid JSON"; }
  }
  return <div className="border-t border-border-subtle pt-4 first:border-0 first:pt-0">
    <div className="flex flex-wrap items-start justify-between gap-2">
      <div>
        <p className="break-all font-mono text-14 font-medium">{env.repo}</p>
        <p className="mt-0.5 text-12 text-fg-tertiary">
          {provenance(env)}
          {env.inferred_from.length > 0 && <> · read from {env.inferred_from.join(", ")}</>}
          {env.learned_from_run_id && <> · last tool from <Link className="underline underline-offset-2" href={`/runs/${env.learned_from_run_id}`}>this run</Link></>}
        </p>
      </div>
      <div className="flex gap-2">
        <Button variant="outline" onClick={() => setDraft(draft === null ? JSON.stringify(env.recipe, null, 2) : null)}>{draft === null ? "Edit" : "Cancel"}</Button>
        <Button variant="outline" disabled={forget.isPending} onClick={() => {
          if (window.confirm(`Forget the test environment for ${env.repo}? Its next run infers it again.`)) forget.mutate();
        }}>Forget</Button>
      </div>
    </div>
    {draft === null ? <div className="mt-3 space-y-2 text-14">
      {services.map((s) => <div key={s.name} className="flex flex-wrap items-center gap-2">
        <SmallPill tone="neutral">{s.name}</SmallPill><code className="text-12 text-fg-secondary">{s.image}</code>
        {Object.keys(s.expose ?? {}).map((k) => <code key={k} className="text-12 text-fg-tertiary">{k}</code>)}
      </div>)}
      {packages.length > 0 && <div className="flex flex-wrap gap-2">{packages.map((p) => <code key={p} className="rounded bg-surface-sunken px-1.5 py-0.5 text-12">{p}</code>)}</div>}
      {!services.length && !packages.length && <p className="text-fg-tertiary">Nothing: runs start with the image as it is.</p>}
    </div> : <div className="mt-3 space-y-2">
      <label htmlFor={`recipe-${env.repo}`} className="sr-only">Test environment for {env.repo} as JSON</label>
      <textarea id={`recipe-${env.repo}`} value={draft} onChange={(e) => setDraft(e.target.value)} rows={12} spellCheck={false}
        className="w-full rounded-md border border-border-strong bg-surface p-3 font-mono text-12 text-fg" />
      {(parseError || save.error) && <p role="alert" className="text-12 text-pill-blocked-fg">{parseError || (save.error instanceof Error ? save.error.message : "Couldn’t save")}</p>}
      <Button disabled={!!parseError || save.isPending} onClick={() => save.mutate(draft)}>{save.isPending ? "Saving…" : "Save"}</Button>
    </div>}
  </div>;
}
