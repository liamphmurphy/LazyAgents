"use client";

import Link from "next/link";
import { DropdownMenu } from "radix-ui";
import { useState, type ReactNode } from "react";
import type { AgentDetail, AgentSpec, Harness, ScopeMode } from "@/lib/api/client";
import { cx } from "@/lib/cx";
import { enforcementFor, MODE_LABEL } from "@/lib/enforcement";
import { Button, IconButton } from "@/components/ui/button";
import { Segmented, Toggle } from "@/components/ui/controls";
import { Icon } from "@/components/ui/icon";
import { Card, Field, TextArea, TextInput } from "@/components/ui/primitives";
import { AGENT_HUES, AgentAvatar } from "@/components/domain/avatars";
import { EnforcementChip, MonoChip } from "@/components/domain/labels";
import { floatPanel } from "@/components/domain/pickers";

export type Draft = { name: string; role: string; specialty: string; color_index: number; spec: AgentSpec };

export function draftOf(d: AgentDetail): Draft {
  return {
    name: d.agent.name,
    role: d.agent.role,
    specialty: d.specialty,
    color_index: d.agent.color_index,
    spec: structuredClone(d.spec),
  };
}

const MODES: Array<{ value: ScopeMode; label: string }> = (["allowed", "requires_approval", "denied"] as const).map((m) => ({
  value: m,
  label: MODE_LABEL[m],
}));

/** A one-line summary of what changed, used as the version note. */
export function describeChanges(a: Draft, b: Draft): string {
  const out: string[] = [];
  if (a.name !== b.name || a.role !== b.role || a.specialty !== b.specialty || a.color_index !== b.color_index) out.push("Updated identity");
  if (a.spec.harness !== b.spec.harness) out.push(`Harness ${a.spec.harness} → ${b.spec.harness}`);
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
  return out.join("; ") || "No changes";
}

const DEFAULT_MEMORY: NonNullable<AgentSpec["memory"]> = { enabled: true, inject: "linked", max_tokens: 4000 };
const INJECT_HELP = {
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

export function DefinitionEditor({ detail, draft, onChange }: { detail: AgentDetail; draft: Draft; onChange: (d: Draft) => void }) {
  const spec = draft.spec;
  const set = (patch: Partial<AgentSpec>) => onChange({ ...draft, spec: { ...spec, ...patch } });
  const harness = detail.harnesses.find((h) => h.name === spec.harness);
  const [newScope, setNewScope] = useState("");
  const [newServer, setNewServer] = useState({ name: "", url: "" });
  const [newOption, setNewOption] = useState({ harness: spec.harness as Harness, key: "", value: "" });
  const scopes = Object.keys(spec.scopes).sort();
  const catalogFree = detail.catalog.filter((c) => !(c.scope in spec.scopes));

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
              <span className="text-fg-secondary">On {harness.label}, LazyAgents can</span>
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
          <Field label="Model" htmlFor="ag-model">
            <TextInput id="ag-model" mono list="ag-models" value={spec.model} onChange={(e) => set({ model: e.target.value })} />
            <datalist id="ag-models">
              {["opus-5.5", "sonnet-5.5", "haiku-5.5", "fable-5.1"].map((m) => (
                <option key={m} value={m} />
              ))}
            </datalist>
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

      <SectionCard title="Instructions" action={<span className="text-xs text-fg-tertiary">Markdown</span>}>
        <TextArea aria-label="Instructions" mono rows={9} value={spec.instructions} onChange={(e) => set({ instructions: e.target.value })} />
        <p className="-mt-2 text-xs text-fg-tertiary">
          Written to <code className="inline">AGENTS.md</code> in the run workspace. If the repo already has one, this goes in a marked section and the repo&apos;s content is kept. Projects can append.
        </p>
      </SectionCard>

      <Card
        title="Permissions"
        bodyClassName="p-0"
        action={
          <DropdownMenu.Root>
            <DropdownMenu.Trigger asChild>
              <Button variant="outline" icon="plus">
                Add scope
              </Button>
            </DropdownMenu.Trigger>
            <DropdownMenu.Portal>
              <DropdownMenu.Content align="end" sideOffset={6} className={cx(floatPanel, "w-[300px]")}>
                {catalogFree.map((c) => (
                  <DropdownMenu.Item
                    key={c.scope}
                    onSelect={() => set({ scopes: { ...spec.scopes, [c.scope]: "requires_approval" } })}
                    className="flex cursor-default flex-col rounded-md px-2 py-1.5 outline-none data-[highlighted]:bg-hover"
                  >
                    <span className="font-mono text-xs">{c.scope}</span>
                    <span className="text-[11.5px] text-fg-tertiary">{c.description}</span>
                  </DropdownMenu.Item>
                ))}
                <form
                  className="mt-1 flex gap-1.5 border-t border-border-subtle p-1.5"
                  onSubmit={(e) => {
                    e.preventDefault();
                    const s = newScope.trim().toLowerCase();
                    if (/^[a-z][a-z0-9-]*:([a-z0-9][a-z0-9-]*|\*)$/.test(s)) {
                      set({ scopes: { ...spec.scopes, [s]: "requires_approval" } });
                      setNewScope("");
                    }
                  }}
                >
                  <TextInput mono aria-label="Custom scope" placeholder="custom:scope" value={newScope} onChange={(e) => setNewScope(e.target.value)} onKeyDown={(e) => e.stopPropagation()} />
                  <Button type="submit" variant="secondary">Add</Button>
                </form>
              </DropdownMenu.Content>
            </DropdownMenu.Portal>
          </DropdownMenu.Root>
        }
      >
        <p className="px-[18px] pt-3 text-xs text-fg-tertiary">
          Projects can only tighten these. &ldquo;Enforced by&rdquo; shows how strongly each holds on {harness?.label}. New scopes start at Approval.
        </p>
        <div className="overflow-x-auto">
          <table className="w-full min-w-[640px] border-collapse text-[13px]">
            <thead>
              <tr className="text-left text-[12.5px] text-fg-tertiary">
                <th className="px-[18px] py-2.5 font-normal">Scope</th>
                <th className="px-2 py-2.5 font-normal">Mode</th>
                <th className="px-2 py-2.5 font-normal">Enforced by</th>
                <th className="px-2 py-2.5 font-normal">How</th>
                <th className="w-10" />
              </tr>
            </thead>
            <tbody>
              {scopes.map((s) => {
                const mode = spec.scopes[s];
                const { enforcement, how } = enforcementFor(s, mode, spec.harness, detail);
                const info = detail.catalog.find((c) => c.scope === s);
                return (
                  <tr key={s} className="border-t border-border-row">
                    <td className="px-[18px] py-2.5">
                      <div className="font-mono text-[12.5px]">{s}</div>
                      {info && <div className="text-xs text-fg-tertiary">{info.description}</div>}
                    </td>
                    <td className="px-2">
                      <Segmented small label={`Mode for ${s}`} value={mode} options={MODES} onChange={(m) => set({ scopes: { ...spec.scopes, [s]: m } })} />
                    </td>
                    <td className="px-2">
                      <EnforcementChip level={enforcement} />
                    </td>
                    <td className="px-2 text-xs text-fg-secondary">{how}</td>
                    <td className="pr-3">
                      <IconButton
                        icon="trash"
                        label={`Remove ${s}`}
                        onClick={() => {
                          const next = { ...spec.scopes };
                          delete next[s];
                          set({ scopes: next });
                        }}
                      />
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
        <div className="flex flex-wrap gap-x-4 gap-y-1.5 border-t border-border-subtle px-[18px] py-3 text-xs text-fg-tertiary">
          <span className="flex items-center gap-1.5"><EnforcementChip level="credential" /> the secret is only in the container when allowed</span>
          <span className="flex items-center gap-1.5"><EnforcementChip level="brokered" /> LazyAgents runs it; the agent never holds the secret</span>
          <span className="flex items-center gap-1.5"><EnforcementChip level="harness" /> best-effort harness tool rules</span>
          <span className="flex items-center gap-1.5"><EnforcementChip level="none" /> instructions only</span>
        </div>
      </Card>

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
