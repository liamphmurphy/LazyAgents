"use client";

import Link from "next/link";
import { useState, type ReactNode } from "react";
import type { AgentDetail, AgentKind, AgentSpec, Harness } from "@/lib/api/client";
import { cx } from "@/lib/cx";
import { MODE_LABEL } from "@/lib/enforcement";
import { Button, IconButton } from "@/components/ui/button";
import { Segmented, Toggle } from "@/components/ui/controls";
import { Icon } from "@/components/ui/icon";
import { Card, Field, TextInput } from "@/components/ui/primitives";
import { AGENT_HUES, AgentAvatar } from "@/components/domain/avatars";
import { MonoChip } from "@/components/domain/labels";

export type Draft = { name: string; role: string; specialty: string; color_index: number; kind: AgentKind; spec: AgentSpec };

export function draftOf(d: AgentDetail): Draft {
  return {
    name: d.agent.name,
    role: d.agent.role,
    specialty: d.specialty,
    color_index: d.agent.color_index,
    kind: d.agent.kind,
    spec: structuredClone(d.spec),
  };
}

/** A one-line summary of what changed, used as the version note. */
export function describeChanges(a: Draft, b: Draft): string {
  const out: string[] = [];
  if (a.name !== b.name || a.role !== b.role || a.specialty !== b.specialty || a.color_index !== b.color_index) out.push("Updated identity");
  if (a.kind !== b.kind) out.push(`Kind ${a.kind} → ${b.kind}`);
  if (a.spec.harness !== b.spec.harness) out.push(`Harness ${a.spec.harness} → ${b.spec.harness}`);
  if (JSON.stringify(a.spec.fallbacks ?? []) !== JSON.stringify(b.spec.fallbacks ?? [])) out.push("Changed usage fallbacks");
  if (a.spec.model !== b.spec.model) out.push(`Model ${a.spec.model} → ${b.spec.model}`);
  if (a.spec.instructions !== b.spec.instructions) out.push("Edited instructions");
  const scopes = new Set([...Object.keys(a.spec.scopes), ...Object.keys(b.spec.scopes)]);
  for (const s of [...scopes].sort()) {
    const x = a.spec.scopes[s];
    const y = b.spec.scopes[s];
    if (x === y) continue;
    if (!x) out.push(`Added ${s}`);
    else if (!y) out.push(`Removed ${s}`);
    else out.push(`${s} → ${y === "requires_approval" ? "Requires approval" : MODE_LABEL[y]}`);
  }
  const names = (s: AgentSpec) => s.mcp_servers.map((m) => m.name).join(",");
  if (names(a.spec) !== names(b.spec)) out.push("Changed MCP servers");
  if (JSON.stringify(a.spec.budget) !== JSON.stringify(b.spec.budget) || a.spec.max_concurrency !== b.spec.max_concurrency) out.push("Changed run limits");
  if (JSON.stringify(a.spec.harness_options) !== JSON.stringify(b.spec.harness_options)) out.push("Changed harness options");
  if (JSON.stringify(a.spec.memory) !== JSON.stringify(b.spec.memory)) out.push("Changed memory");
  const sk = (s: AgentSpec) => new Map((s.skills ?? []).map((k) => [k.name, k]));
  const [sa, sb] = [sk(a.spec), sk(b.spec)];
  for (const [n, k] of sb) {
    if (!sa.has(n)) out.push(`Added skill ${n}`);
    else if (JSON.stringify(sa.get(n)) !== JSON.stringify(k)) out.push(`Edited skill ${n}`);
  }
  for (const n of sa.keys()) if (!sb.has(n)) out.push(`Removed skill ${n}`);
  return out.join("; ") || "No changes";
}

const MODEL_SUGGESTIONS: Record<Harness, string[]> = {
  "claude-code": ["opus-5.5", "sonnet-5.5", "haiku-5.5", "fable-5.1"],
  codex: ["gpt-5-codex", "gpt-5"],
  opencode: ["anthropic/claude-sonnet-5-5", "anthropic/claude-opus-5-5", "openai/gpt-5"],
};
const OPENCODE_HINT = "Use provider/model, e.g. anthropic/claude-sonnet-5-5";

const DEFAULT_MEMORY: NonNullable<AgentSpec["memory"]> = { enabled: true, inject: "selective", max_tokens: 1500 };
const INJECT_HELP = {
  selective: "Up to three relevant previews; load full pages only when needed.",
  index: "Only the agent's Memory page.",
  linked: "The Memory page, plus pages linked to or from the ticket and its documents.",
  search: "Everything in Linked, plus the best wiki matches for the ticket title.",
} as const;

/** Wiki memory a run starts with (PLAN.md §12.3). */
function MemoryCard({ handle, spec, set }: { handle: string; spec: AgentSpec; set: (p: Partial<AgentSpec>) => void }) {
  const m = spec.memory ?? DEFAULT_MEMORY;
  const put = (p: Partial<typeof m>) => set({ memory: { ...m, ...p } });
  return (
    <SectionCard
      title="Memory"
      action={
        <Link href={`/wiki/agents/${handle}/Memory`} className="text-xs text-accent hover:underline">
          Open memory
        </Link>
      }
    >
      <Toggle checked={m.enabled} onChange={(enabled) => put({ enabled })} label="Start each run with wiki context" />
      {m.enabled && (
        <div className="grid grid-cols-[repeat(auto-fit,minmax(200px,1fr))] gap-4">
          <div className="flex flex-col gap-1.5">
            <span className="text-[12.5px] font-medium text-fg-2">Inject</span>
            <Segmented
              label="Inject"
              value={m.inject}
              onChange={(inject) => put({ inject })}
              options={[
                { value: "selective", label: "Selective" },
                { value: "index", label: "Index" },
                { value: "linked", label: "Linked" },
                { value: "search", label: "Search" },
              ]}
            />
            <span className="text-xs text-fg-tertiary">{INJECT_HELP[m.inject]}</span>
          </div>
          <Field label="Token budget" htmlFor="ag-mem-tokens" hint="Most relevant pages first; the rest is trimmed.">
            <TextInput
              id="ag-mem-tokens"
              mono
              type="number"
              min={500}
              max={50000}
              step={500}
              value={m.max_tokens}
              onChange={(e) => put({ max_tokens: Number(e.target.value) || DEFAULT_MEMORY.max_tokens })}
            />
          </Field>
        </div>
      )}
      <p className="-mt-1 text-xs text-fg-tertiary">
        The agent always writes to <code className="inline">agents/{handle}/</code>. Edits to other pages follow the{" "}
        <code className="inline">wiki:write</code> scope; without it they arrive as suggestions.
      </p>
    </SectionCard>
  );
}

function SectionCard({ title, action, children, bodyClassName }: { title: string; action?: ReactNode; children: ReactNode; bodyClassName?: string }) {
  return (
    <Card title={title} action={action} bodyClassName={cx("flex flex-col gap-4 px-[18px] py-4", bodyClassName)}>
      {children}
    </Card>
  );
}

const CAP_LABELS: Array<[keyof AgentDetail["harnesses"][number], string]> = [
  ["tool_gating", "gate individual tools"],
  ["permission_prompt", "route permission prompts here"],
  ["native_cost", "read cost from the harness"],
  ["resume", "resume sessions"],
];

/** Settings: identity, runtime, memory, MCP servers, fallbacks and harness options. Instructions, scopes and skills have their own tabs. */
export function DefinitionEditor({ detail, draft, onChange }: { detail: AgentDetail; draft: Draft; onChange: (d: Draft) => void }) {
  const spec = draft.spec;
  const set = (patch: Partial<AgentSpec>) => onChange({ ...draft, spec: { ...spec, ...patch } });
  const harness = detail.harnesses.find((h) => h.name === spec.harness);
  const [newServer, setNewServer] = useState({ name: "", url: "" });
  const [newOption, setNewOption] = useState({ harness: spec.harness as Harness, key: "", value: "" });

  return (
    <div className="flex flex-col gap-4">
      <SectionCard title="Identity">
        <div className="grid grid-cols-[repeat(auto-fit,minmax(220px,1fr))] gap-4">
          <Field label="Name" htmlFor="ag-name">
            <TextInput id="ag-name" value={draft.name} maxLength={40} onChange={(e) => onChange({ ...draft, name: e.target.value })} />
          </Field>
          <Field label="Handle" htmlFor="ag-handle" hint="Used for @mentions and branch names. Fixed once created.">
            <TextInput id="ag-handle" mono prefix="@" value={detail.agent.handle} readOnly className="text-fg-secondary" />
          </Field>
        </div>
        <Field label="Role" htmlFor="ag-role">
          <TextInput id="ag-role" value={draft.role} onChange={(e) => onChange({ ...draft, role: e.target.value })} placeholder="e.g. Backend · migrations & data models" />
        </Field>
        <Field label="Specialty" htmlFor="ag-spec" hint="Shown in the agent picker next to the model">
          <TextInput id="ag-spec" value={draft.specialty} onChange={(e) => onChange({ ...draft, specialty: e.target.value })} />
        </Field>
        <div className="flex flex-col gap-1.5">
          <span className="text-[12.5px] font-medium text-fg-2">Kind</span>
          <Segmented
            label="Agent kind"
            value={draft.kind}
            onChange={(kind) => onChange({ ...draft, kind })}
            options={[
              { value: "main", label: "Main" },
              { value: "sub", label: "Sub" },
            ]}
          />
          <p className="text-xs text-fg-tertiary">
            {draft.kind === "main"
              ? "Takes tickets, and can delegate parts of them to sub agents."
              : "Can't be assigned tickets. Runs only when a main agent delegates a task to it."}
          </p>
        </div>
        <div className="flex flex-col gap-1.5">
          <span className="text-[12.5px] font-medium text-fg-2">Identity color</span>
          <div role="radiogroup" aria-label="Identity color" className="flex gap-1.5">
            {AGENT_HUES.map((hue, i) => (
              <button
                key={hue}
                type="button"
                role="radio"
                aria-checked={draft.color_index === i}
                aria-label={hue}
                onClick={() => onChange({ ...draft, color_index: i })}
                className={cx("rounded-[7px] p-0.5", draft.color_index === i ? "ring-2 ring-accent" : "hover:ring-1 hover:ring-border-strong")}
              >
                <AgentAvatar name={draft.name || "?"} colorIndex={i} size="md" label={false} />
              </button>
            ))}
          </div>
        </div>
      </SectionCard>

      <SectionCard title="Runtime">
        <Toggle checked={spec.review_required ?? false} onChange={(review_required) => set({ review_required })} label="Require independent review before Done" />
        <div className="flex flex-col gap-2">
          <span className="text-[12.5px] font-medium text-fg-2">Harness</span>
          <div className="flex flex-wrap items-center gap-3">
            <Segmented
              label="Harness"
              value={spec.harness}
              onChange={(h) => set({ harness: h })}
              options={detail.harnesses.map((h) => ({ value: h.name, label: h.label }))}
            />
            <span className="text-xs text-fg-tertiary">Projects can switch harness in an override</span>
          </div>
          {harness && (
            <div className="rounded-lg border border-border bg-surface-sunken px-3 py-2.5 text-[12.5px]">
              <span className="text-fg-secondary">On {harness.label}, Vesper can</span>
              <ul className="mt-1 flex flex-wrap gap-x-4 gap-y-1">
                {CAP_LABELS.map(([k, label]) => (
                  <li key={k} className={cx("flex items-center gap-1.5", harness[k] ? "text-fg-2" : "text-fg-tertiary line-through")}>
                    <Icon name={harness[k] ? "check" : "x"} size={12} className={harness[k] ? "text-state-done" : ""} />
                    {label}
                  </li>
                ))}
              </ul>
            </div>
          )}
        </div>
        <div className="grid grid-cols-[repeat(auto-fit,minmax(160px,1fr))] gap-4">
          <Field label="Model" htmlFor="ag-model" hint={spec.harness === "opencode" ? OPENCODE_HINT : undefined}>
            <TextInput id="ag-model" mono list="ag-models" value={spec.model} onChange={(e) => set({ model: e.target.value })} />
            <datalist id="ag-models">
              {MODEL_SUGGESTIONS[spec.harness].map((m) => (
                <option key={m} value={m} />
              ))}
            </datalist>
          </Field>
          <Field label="Effort" htmlFor="ag-effort" hint="Reasoning effort. Default uses the harness's own default.">
            <select
              id="ag-effort"
              className="h-[30px] w-full rounded-md border border-border-strong bg-surface px-2 text-[13px] text-fg focus:border-accent focus:outline-none"
              value={spec.harness_options[spec.harness]?.effort ?? ""}
              onChange={(e) => {
                const rest = { ...spec.harness_options[spec.harness] };
                delete rest.effort;
                const next = { ...spec.harness_options };
                const opts = e.target.value ? { ...rest, effort: e.target.value } : rest;
                if (Object.keys(opts).length) next[spec.harness] = opts;
                else delete next[spec.harness];
                set({ harness_options: next });
              }}
            >
              <option value="">Default</option>
              {["low", "medium", "high", "xhigh", "max"].map((v) => (
                <option key={v} value={v}>{v}</option>
              ))}
            </select>
          </Field>
          <Field label="Concurrent runs" htmlFor="ag-conc">
            <TextInput id="ag-conc" type="number" min={1} max={16} value={spec.max_concurrency} onChange={(e) => set({ max_concurrency: Number(e.target.value) || 1 })} />
          </Field>
          <Field label="Run guards" htmlFor="ag-runtime" hint="Optional. Stops runaway runs. No spend caps in v1.">
            <div className="flex gap-2">
              <TextInput id="ag-runtime" mono aria-label="Max runtime" placeholder="2h" value={spec.budget.max_runtime ?? ""} onChange={(e) => set({ budget: { ...spec.budget, max_runtime: e.target.value || undefined } })} />
              <TextInput
                mono
                aria-label="Max turns"
                type="number"
                min={0}
                placeholder="turns"
                value={spec.budget.max_turns ?? ""}
                onChange={(e) => set({ budget: { ...spec.budget, max_turns: e.target.value ? Number(e.target.value) : undefined } })}
              />
            </div>
          </Field>
        </div>
      </SectionCard>

      <MemoryCard handle={detail.agent.handle} spec={spec} set={set} />

      <Card title="MCP servers" bodyClassName="p-0">
        <ul>
          <li className="flex items-center gap-3 px-[18px] py-2.5 text-[13px]">
            <Icon name="server" size={14} className="text-fg-tertiary" />
            <span className="font-mono">lazyagents</span>
            <span className="truncate font-mono text-xs text-fg-tertiary">/mcp</span>
            <span className="ml-auto flex items-center gap-1.5 text-xs text-fg-tertiary">
              <Icon name="lock" size={12} /> Always on · per-run token
            </span>
          </li>
          {spec.mcp_servers.map((m, i) => (
            <li key={m.name} className="flex items-center gap-3 border-t border-border-row px-[18px] py-2 text-[13px]">
              <Icon name="server" size={14} className="text-fg-tertiary" />
              <span className="font-mono">{m.name}</span>
              <span className="min-w-0 truncate font-mono text-xs text-fg-tertiary">{m.url}</span>
              <span className="ml-auto truncate text-xs text-fg-tertiary">{m.description}</span>
              <IconButton icon="trash" label={`Remove ${m.name}`} onClick={() => set({ mcp_servers: spec.mcp_servers.filter((_, j) => j !== i) })} />
            </li>
          ))}
        </ul>
        <form
          className="flex flex-wrap gap-2 border-t border-border-subtle px-[18px] py-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (!newServer.name.trim() || !newServer.url.trim()) return;
            set({ mcp_servers: [...spec.mcp_servers, { name: newServer.name.trim(), url: newServer.url.trim() }] });
            setNewServer({ name: "", url: "" });
          }}
        >
          <TextInput mono aria-label="Server name" placeholder="name" className="w-40" value={newServer.name} onChange={(e) => setNewServer({ ...newServer, name: e.target.value })} />
          <TextInput mono aria-label="Server URL" placeholder="https://…/mcp" className="min-w-[200px] flex-1" value={newServer.url} onChange={(e) => setNewServer({ ...newServer, url: e.target.value })} />
          <Button type="submit" variant="outline" icon="plus">Add server</Button>
        </form>
      </Card>

      <SectionCard title="Usage fallbacks">
        <p className="text-xs text-fg-tertiary">When provider usage runs out, continue with the next harness and model in this order. The checkout is preserved; each switch starts a fresh session. Configure credentials for each harness.</p>
        {(spec.fallbacks ?? []).map((f, i) => (
          <div key={i} className="flex flex-wrap items-center gap-2">
            <span className="text-xs text-fg-tertiary">{i + 1}.</span>
            <Segmented label={`Fallback ${i + 1} harness`} value={f.harness} options={detail.harnesses.map((h) => ({ value: h.name, label: h.label }))}
              onChange={(harness) => set({ fallbacks: spec.fallbacks!.map((v, j) => j === i ? { ...v, harness, harness_options: v.harness_options?.effort ? { effort: v.harness_options.effort } : {} } : v) })} />
            <TextInput aria-label={`Fallback ${i + 1} model`} placeholder={f.harness === "opencode" ? "provider/model" : "Model"} list={`ag-fb-models-${i}`} value={f.model}
              onChange={(e) => set({ fallbacks: spec.fallbacks!.map((v, j) => j === i ? { ...v, model: e.target.value } : v) })} />
            <select
              aria-label={`Fallback ${i + 1} effort`}
              className="h-[30px] rounded-md border border-border-strong bg-surface px-2 text-[13px] text-fg focus:border-accent focus:outline-none"
              value={f.harness_options?.effort ?? ""}
              onChange={(e) => set({
                fallbacks: spec.fallbacks!.map((v, j) => {
                  if (j !== i) return v;
                  const opts = { ...v.harness_options };
                  delete opts.effort;
                  return { ...v, harness_options: e.target.value ? { ...opts, effort: e.target.value } : opts };
                }),
              })}
            >
              <option value="">Default effort</option>
              {["low", "medium", "high", "xhigh", "max"].map((v) => (
                <option key={v} value={v}>{v}</option>
              ))}
            </select>
            <datalist id={`ag-fb-models-${i}`}>
              {MODEL_SUGGESTIONS[f.harness].map((m) => (
                <option key={m} value={m} />
              ))}
            </datalist>
            <IconButton icon="trash" label={`Remove fallback ${i + 1}`} onClick={() => set({ fallbacks: spec.fallbacks!.filter((_, j) => j !== i) })} />
          </div>
        ))}
        <Button type="button" variant="outline" disabled={(spec.fallbacks?.length ?? 0) >= 8}
          onClick={() => set({ fallbacks: [...(spec.fallbacks ?? []), { harness: detail.harnesses.find((h) => h.name !== spec.harness)?.name ?? spec.harness, model: "" }] })}>Add fallback</Button>
      </SectionCard>

      <Card title="Harness options" bodyClassName="flex flex-col gap-2 px-[18px] py-4">
        <p className="text-xs text-fg-tertiary">Passed only to the matching harness. Use for settings that have no shared equivalent.</p>
        {Object.entries(spec.harness_options).flatMap(([h, opts]) =>
          Object.entries(opts).map(([k, v]) => (
            <div key={`${h}.${k}`} className="flex items-center gap-2">
              <span className="w-[92px] flex-none"><MonoChip variant="outline">{h}</MonoChip></span>
              <span className="w-44 flex-none truncate font-mono text-xs">{k}</span>
              <TextInput
                aria-label={`${h} ${k}`}
                value={v}
                className="flex-1"
                onChange={(e) => set({ harness_options: { ...spec.harness_options, [h]: { ...opts, [k]: e.target.value } } })}
              />
              <IconButton
                icon="trash"
                label={`Remove ${h} ${k}`}
                onClick={() => {
                  const rest = { ...opts };
                  delete rest[k];
                  const next = { ...spec.harness_options, [h]: rest };
                  if (!Object.keys(rest).length) delete next[h];
                  set({ harness_options: next });
                }}
              />
            </div>
          )),
        )}
        <form
          className="flex flex-wrap items-center gap-2 border-t border-border-subtle pt-3"
          onSubmit={(e) => {
            e.preventDefault();
            const k = newOption.key.trim();
            if (!k) return;
            const cur = spec.harness_options[newOption.harness] ?? {};
            set({ harness_options: { ...spec.harness_options, [newOption.harness]: { ...cur, [k]: newOption.value } } });
            setNewOption({ ...newOption, key: "", value: "" });
          }}
        >
          <Segmented small label="Option harness" value={newOption.harness} onChange={(h) => setNewOption({ ...newOption, harness: h })} options={detail.harnesses.map((h) => ({ value: h.name, label: h.name }))} />
          <TextInput mono aria-label="Option key" placeholder="key" className="w-44" value={newOption.key} onChange={(e) => setNewOption({ ...newOption, key: e.target.value })} />
          <TextInput aria-label="Option value" placeholder="value" className="min-w-[140px] flex-1" value={newOption.value} onChange={(e) => setNewOption({ ...newOption, value: e.target.value })} />
          <Button type="submit" variant="outline" icon="plus">Add option</Button>
        </form>
      </Card>
    </div>
  );
}
