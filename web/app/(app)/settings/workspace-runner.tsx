"use client";

import { useBootstrap, useRunnerStatus, useSecrets } from "@/lib/api/hooks";
import { Button, ButtonLink } from "@/components/ui/button";
import { Banner, Card, Skeleton } from "@/components/ui/primitives";

export function WorkspaceRunner() {
  const boot = useBootstrap();
  const secrets = useSecrets();
  const runner = useRunnerStatus();
  return <div className="space-y-6">
    <section id="workspace">
      <Card title="Workspace">
        <p className="mb-3 text-sm text-fg-secondary">Shared by every project. Encryption is configured on the API server.</p>
        {boot.isPending && <Skeleton className="h-16" />}
        {boot.isError && <Banner tone="blocked" title="Couldn’t load workspace" actions={<Button onClick={() => void boot.refetch()}>Retry</Button>}>Workspace details are unavailable.</Banner>}
        {boot.data?.workspace ? <dl className="grid grid-cols-2 gap-2 text-sm"><dt>Name</dt><dd>{boot.data.workspace.name}</dd><dt>Slug</dt><dd>{boot.data.workspace.slug}</dd></dl> : boot.isSuccess && <p>No workspace configured.</p>}
        <p className="mt-3 text-sm">{secrets.isPending ? "Checking secret encryption…" : secrets.isError ? "Secret encryption status unavailable. Retry in Provider secrets." : secrets.data?.key_configured ? "SECRETS_KEY is configured." : "SECRETS_KEY is missing: secrets are read-only and Docker runs cannot receive provider keys."}</p>
      </Card>
    </section>
    <section id="runner">
      <Card title="Runner backends & concurrency">
        <p className="mb-3 text-sm text-fg-secondary">RUNNER_BACKEND controls where runs execute: local uses host tools and CLI logins; Docker isolates each run in a container. RUNNER_MAX_CONCURRENT limits simultaneous runs per runner. Change these on the runner and restart it.</p>
        {runner.isPending && <Skeleton className="h-20" />}
        {runner.isError && <Banner tone="blocked" title="Couldn’t load runners" actions={<Button onClick={() => void runner.refetch()}>Retry</Button>}>Current backend and concurrency are unknown.</Banner>}
        {runner.data && (runner.data.runners.length ? <ul className="space-y-3">{runner.data.runners.map((r) => <li key={r.id} className="text-sm"><span className="font-mono">{r.id}</span> · {r.backend} · concurrency {r.max_concurrent} · {r.running} running</li>)}</ul> : <p>No runners online. Start a runner to execute dispatched work; current settings appear after its heartbeat.</p>)}
        <div className="mt-4"><ButtonLink href="/settings/runner" variant="outline">Inspect runner environment</ButtonLink></div>
      </Card>
    </section>
  </div>;
}
