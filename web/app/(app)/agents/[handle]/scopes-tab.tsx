"use client";

import { useState } from "react";
import type { AgentDetail, Schemas, ScopeMode } from "@/lib/api/client";
import { scopeRows, type Mode, type ScopeRow } from "@/lib/agent-scopes";
import { cx } from "@/lib/cx";
import { enforcementFor, MODE_LABEL } from "@/lib/enforcement";
import { Button, IconButton } from "@/components/ui/button";
import { Segmented } from "@/components/ui/controls";
import { TextInput } from "@/components/ui/primitives";
import { EnforcementChip, MonoChip } from "@/components/domain/labels";
import type { Draft } from "./definition-editor";
import { ProjectPicker } from "./project-picker";

const LEVELS: Array<{ value: ScopeMode; label: string }> = (["allowed", "requires_approval", "denied"] as const).map((m) => ({ value: m, label: MODE_LABEL[m] }));
const SCOPE_NAME = /^[a-z][a-z0-9-]*:([a-z0-9][a-z0-9-]*|\*)$/;

const LEVEL_CLASS: Record<Mode, string> = {
  allowed: "bg-pill-done-bg text-pill-done-fg",
  requires_approval: "bg-pill-human-bg text-pill-human-fg",
  denied: "bg-pill-blocked-bg text-pill-blocked-fg",
};

export function LevelTag({ mode }: { mode: Mode }) {
  return <span className={cx("inline-flex h-[22px] items-center rounded-[5px] px-2 text-xs font-medium", LEVEL_CLASS[mode])}>{MODE_LABEL[mode]}</span>;
}

/** One plain-language line per scope: the catalog's, or a description of the custom name. */
export function scopeNote(scope: string, detail: Pick<AgentDetail, "catalog">, project?: string): string {
  const info = detail.catalog.find((c) => c.scope === scope);
  if (info) return info.description;
  if (scope.endsWith(":*")) return `Custom wildcard: covers every ${scope.slice(0, -1)} scope${project ? `, named by the ${project} override` : ""}`;
  return "Custom scope: enforced only through the agent's instructions unless a secret grants it";
}

/** The agent's scopes as a matrix: definition level, the effective level in a project, and how it's enforced. */
export function ScopesTab({
  detail,
  draft,
  onChange,
  projects,
  project,
  onProject,
}: {
  detail: AgentDetail;
  draft: Draft;
  onChange: (d: Draft) => void;
  projects: Schemas["Project"][];
  project?: string;
  onProject: (key: string) => void;
}) {
  const [custom, setCustom] = useState("");
  const spec = draft.spec;
  const saved = detail.spec.scopes;
  const p = projects.find((x) => x.key === project);
  const override = detail.overrides.find((o) => o.project.key === project)?.override;
  const harness = override?.harness ?? spec.harness;
  const rows = scopeRows(detail.catalog.map((c) => c.scope), spec.scopes, override?.scopes);
  const setScope = (scope: string, mode: ScopeMode | undefined) => {
    const next = { ...spec.scopes };
    if (mode) next[scope] = mode;
    else delete next[scope];
    onChange({ ...draft, spec: { ...spec, scopes: next } });
  };
  const customOk = SCOPE_NAME.test(custom.trim().toLowerCase()) && !(custom.trim().toLowerCase() in spec.scopes);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <span className="text-[12.5px] text-fg-tertiary">Effective in</span>
        <ProjectPicker label="Effective in" projects={projects} value={project} onChange={onProject} className="w-48" />
        <span className="text-xs text-fg-tertiary">follows the project you&apos;re in · switch to compare</span>
        <form
          className="ml-auto flex items-center gap-1.5"
          onSubmit={(e) => {
            e.preventDefault();
            if (!customOk) return;
            setScope(custom.trim().toLowerCase(), "requires_approval");
            setCustom("");
          }}
        >
          <TextInput mono aria-label="Custom scope" placeholder="custom:scope" className="w-40" value={custom} onChange={(e) => setCustom(e.target.value)} />
          <Button type="submit" variant="outline" icon="plus" disabled={!customOk}>
            Add custom scope
          </Button>
        </form>
      </div>
      <p className="text-xs text-fg-tertiary">
        Pick a level to change the agent&apos;s definition. Projects can only tighten it, and a scope the agent doesn&apos;t grant is denied. New custom scopes start at Approval.
      </p>

      <div className="overflow-x-auto rounded-xl border border-border bg-surface">
        <table className="w-full min-w-[720px] border-collapse text-[13px]">
          <thead>
            <tr className="text-left text-[12px] text-fg-tertiary">
              <th className="px-4 py-2.5 font-normal">Scope</th>
              <th className="px-2 py-2.5 font-normal">Agent definition</th>
              <th className="px-2 py-2.5 font-normal">Effective{p ? ` in ${p.name}` : ""}</th>
              <th className="px-2 py-2.5 pr-4 font-normal">Enforced by</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <Row key={r.scope} row={r} saved={saved[r.scope]} detail={detail} harness={harness} project={p?.name} onSet={(m) => setScope(r.scope, m)} />
            ))}
          </tbody>
        </table>
        <div className="flex flex-wrap gap-x-4 gap-y-1.5 border-t border-border-subtle px-4 py-3 text-xs text-fg-tertiary">
          <span className="flex items-center gap-1.5"><LevelTag mode="allowed" /> no prompt</span>
          <span className="flex items-center gap-1.5"><LevelTag mode="requires_approval" /> each use waits for a person</span>
          <span className="flex items-center gap-1.5"><LevelTag mode="denied" /> never</span>
          <span className="flex items-center gap-1.5"><MonoChip variant="outline">project</MonoChip> set by this project&apos;s override · hover a chip for how it&apos;s enforced</span>
        </div>
      </div>
      <p className="text-xs text-fg-tertiary">Edits stay in this browser until you review and save them. Each run keeps the definition it started with.</p>
    </div>
  );
}

function Row({
  row: r,
  saved,
  detail,
  harness,
  project,
  onSet,
}: {
  row: ScopeRow;
  saved?: Mode;
  detail: AgentDetail;
  harness: AgentDetail["spec"]["harness"];
  project?: string;
  onSet: (m: ScopeMode | undefined) => void;
}) {
  const changed = r.definition !== saved;
  const { enforcement, how } = enforcementFor(r.scope, r.effective.mode, harness, detail);
  const fromProject = r.effective.source === "project";
  return (
    <tr className={cx("border-t border-border-row align-top", changed && "bg-pill-human-bg/30")}>
      <td className="px-4 py-2.5">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-mono text-[12.5px]">{r.scope}</span>
          {changed && <span className="text-[11px] text-pill-human-fg">was {saved ? MODE_LABEL[saved] : "not granted"}</span>}
        </div>
        <div className="text-xs text-fg-tertiary">{scopeNote(r.scope, detail, r.projectOnly ? project : undefined)}</div>
      </td>
      <td className="px-2 py-2.5">
        {r.projectOnly ? (
          <span className="text-xs text-fg-tertiary">not in the definition</span>
        ) : (
          <div className="flex items-center gap-1 whitespace-nowrap">
            <Segmented small label={`Definition level for ${r.scope}`} value={r.definition} options={LEVELS} onChange={(m) => onSet(m)} />
            {r.definition ? (
              <IconButton icon="x" size={24} label={`Remove ${r.scope} from the definition`} onClick={() => onSet(undefined)} />
            ) : (
              <span className="text-[11.5px] text-fg-tertiary">{r.granted ? "via wildcard" : "not granted"}</span>
            )}
          </div>
        )}
      </td>
      <td className="px-2 py-2.5">
        <div className="flex flex-wrap items-center gap-1.5">
          <LevelTag mode={r.effective.mode} />
          {fromProject && <MonoChip variant="outline">project</MonoChip>}
          {changed && !fromProject && <span className="text-[11px] text-fg-tertiary">after save</span>}
        </div>
        {fromProject && project && (
          <div className="mt-0.5 text-[11.5px] text-fg-tertiary">
            {r.projectOnly || !r.granted ? `${project} denies it` : `${project} tightens ${r.definition ? MODE_LABEL[r.definition] : "the agent's level"}`}
          </div>
        )}
      </td>
      <td className="px-2 py-2.5 pr-4">
        <span title={how}>
          <EnforcementChip level={enforcement} />
        </span>
      </td>
    </tr>
  );
}
