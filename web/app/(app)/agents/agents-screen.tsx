"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { useBootstrap, useCreateAgent } from "@/lib/api/hooks";
import { Button } from "@/components/ui/button";
import { Field, Skeleton, TextInput } from "@/components/ui/primitives";
import { AgentAvatar } from "@/components/domain/avatars";
import { MonoChip, Pill, type PillTone } from "@/components/domain/labels";
import { Content, TopBar } from "@/components/shell/topbar";

const STATUS: Record<string, { tone: PillTone; label: string }> = {
  running: { tone: "done", label: "Running" },
  waiting: { tone: "human", label: "Waiting" },
  blocked: { tone: "blocked", label: "Blocked" },
  idle: { tone: "neutral", label: "Idle" },
};

/**
 * A handle from a name, valid for the server's rule (2–32 lowercase letters,
 * digits or -, starting with a letter): "Scout 2" → "scout-2".
 */
function handleFrom(name: string) {
  const h = name
    .toLowerCase()
    .normalize("NFKD")
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^[^a-z]+/, "")
    .slice(0, 32)
    .replace(/-+$/, "");
  return h.length >= 2 ? h : "";
}

/** Minimal roster until Phase 7 adds live assignments, stats and queues. */
export function AgentsScreen() {
  const boot = useBootstrap();
  const router = useRouter();
  const create = useCreateAgent();
  const [adding, setAdding] = useState(false);
  const [form, setForm] = useState({ name: "", handle: "" });
  const agents = boot.data?.agents ?? [];
  // An empty Handle field means "use the one derived from the name", which
  // is what its placeholder shows.
  const handle = form.handle.trim() || handleFrom(form.name);

  return (
    <>
      <TopBar
        crumbs={[{ label: boot.data?.workspace.name ?? "Workspace" }, { label: "Agents" }]}
        actions={<Button variant="primary" icon="plus" onClick={() => setAdding(true)}>New agent</Button>}
      />
      <Content>
        <div className="flex flex-col gap-6">
          <div className="flex flex-col gap-1">
            <h1 className="type-display">Agents</h1>
            <p className="text-sm text-fg-secondary">Definitions, permissions and per-project overrides. Live status and stats arrive in Phase 7.</p>
          </div>
          {adding && (
            <form
              className="flex flex-wrap items-end gap-3 rounded-xl border border-border bg-surface p-4"
              onSubmit={(e) => {
                e.preventDefault();
                create.mutate(
                  { name: form.name.trim(), handle, color_index: agents.length % 8 },
                  { onSuccess: (d) => router.push(`/agents/${d.agent.handle}`) },
                );
              }}
            >
              <Field label="Name" htmlFor="na-name">
                <TextInput id="na-name" autoFocus value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="Scout" />
              </Field>
              <Field label="Handle" htmlFor="na-handle">
                <TextInput id="na-handle" mono prefix="@" value={form.handle} onChange={(e) => setForm({ ...form, handle: e.target.value })} placeholder={handleFrom(form.name) || "scout"} />
              </Field>
              <Button type="submit" variant="primary" disabled={!form.name.trim() || !handle || create.isPending}>Create</Button>
              <Button variant="ghost" onClick={() => setAdding(false)}>Cancel</Button>
              {create.isError && <span role="alert" className="w-full text-xs text-pill-blocked-fg">{create.error.message}</span>}
            </form>
          )}
          <div className="grid grid-cols-[repeat(auto-fill,minmax(300px,1fr))] gap-4">
            {!boot.data
              ? [0, 1, 2].map((i) => <Skeleton key={i} className="h-28 rounded-xl" />)
              : agents.map((a) => (
                  <Link key={a.id} href={`/agents/${a.handle}`} className="flex flex-col gap-3 rounded-xl border border-border bg-surface p-4 hover:border-border-hover">
                    <div className="flex items-center gap-3">
                      <AgentAvatar name={a.name} colorIndex={a.color_index} size="lg" label={false} />
                      <div className="flex min-w-0 flex-1 flex-col">
                        <span className="type-heading">{a.name}</span>
                        <span className="truncate text-xs text-fg-secondary">{a.role}</span>
                      </div>
                      <Pill tone={STATUS[a.status].tone}>{STATUS[a.status].label}</Pill>
                    </div>
                    <div className="flex items-center gap-2 text-xs text-fg-tertiary">
                      <MonoChip variant="outline">{a.model}</MonoChip>
                      <span className="truncate">{a.specialty}</span>
                    </div>
                  </Link>
                ))}
          </div>
        </div>
      </Content>
    </>
  );
}
