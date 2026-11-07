"use client";

import { useState } from "react";
import type { AgentDetail, AgentSpec, Schemas } from "@/lib/api/client";
import { useResolved } from "@/lib/api/hooks";
import { skillViews } from "@/lib/agent-scopes";
import { cx } from "@/lib/cx";
import { Button, IconButton } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { Card, Field, TextArea, TextInput } from "@/components/ui/primitives";
import { MonoChip } from "@/components/domain/labels";
import type { Draft } from "./definition-editor";
import { ProjectPicker } from "./project-picker";

const SKILL_NAME = /^[a-z0-9][a-z0-9-]{0,63}$/;

/**
 * Playbooks loaded on demand. Agent skills are edited here; what a project's
 * override adds or removes is shown from the server's resolved definition and
 * is read-only here; edit additions and removals in Project overrides.
 */
export function SkillsTab({
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
  const spec = draft.spec;
  const skills = spec.skills ?? [];
  const set = (next: NonNullable<AgentSpec["skills"]>) => onChange({ ...draft, spec: { ...spec, skills: next } });
  const resolved = useResolved(detail.agent.handle, project);
  const p = projects.find((x) => x.key === project);
  const views = skillViews(skills, (detail.spec.skills ?? []).map((k) => k.name), resolved.data?.skills);
  const added = views.filter((v) => v.origin === "added").length;
  const removed = views.filter((v) => v.origin === "removed").length;
  const [open, setOpen] = useState<string | null>(null);
  const [name, setName] = useState("");
  const nameOk = SKILL_NAME.test(name) && !skills.some((k) => k.name === name);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <span className="text-[12.5px] text-fg-tertiary">Showing for</span>
        <ProjectPicker label="Showing skills for" projects={projects} value={project} onChange={onProject} className="w-48" />
        <span className="text-xs text-fg-tertiary">
          {skills.length} from the agent{p && ` · ${added} added · ${removed} removed`}
        </span>
      </div>
      <Card bodyClassName="p-0">
        {views.length === 0 && <p className="px-[18px] py-4 text-sm text-fg-tertiary">No skills yet. Add one below.</p>}
        <ul>
          {views.map(({ skill: k, origin }, idx) => {
            const i = skills.indexOf(k);
            const editable = origin === "agent";
            const isOpen = editable && open === k.name;
            return (
              <li key={`${origin}-${k.name}`} className={cx(idx > 0 && "border-t border-border-row")}>
                <div className="flex items-center gap-3 px-[18px] py-2.5 text-[13px]">
                  <Icon name="wiki" size={14} className="text-fg-tertiary" />
                  {editable ? (
                    <button type="button" aria-expanded={isOpen} onClick={() => setOpen(isOpen ? null : k.name)} className="flex min-w-0 flex-1 flex-col text-left">
                      <span className="font-mono text-[12.5px]">{k.name}</span>
                      <span className="truncate text-xs text-fg-tertiary">{k.description || "No description yet"}</span>
                    </button>
                  ) : (
                    <div className="flex min-w-0 flex-1 flex-col">
                      <span className={cx("font-mono text-[12.5px]", origin === "removed" && "text-fg-tertiary line-through")}>{k.name}</span>
                      <span className="truncate text-xs text-fg-tertiary">{k.description}</span>
                    </div>
                  )}
                  {origin === "agent" && <MonoChip variant="outline">agent</MonoChip>}
                  {origin === "removed" && <span className="rounded-[5px] bg-pill-blocked-bg px-[7px] py-0.5 text-[11px] text-pill-blocked-fg">removed in {p?.name}</span>}
                  {origin === "added" && <span className="rounded-[5px] bg-pill-done-bg px-[7px] py-0.5 text-[11px] text-pill-done-fg">added by {p?.name}</span>}
                  {editable && <IconButton icon="trash" label={`Remove skill ${k.name}`} onClick={() => set(skills.filter((_, j) => j !== i))} />}
                </div>
                {isOpen && (
                  <div className="flex flex-col gap-3 px-[18px] pb-4">
                    <Field label="Use when…" htmlFor={`sk-desc-${i}`} hint="One line the agent matches tasks against.">
                      <TextInput id={`sk-desc-${i}`} value={k.description} maxLength={1024} onChange={(e) => set(skills.map((s, j) => (j === i ? { ...s, description: e.target.value } : s)))} />
                    </Field>
                    <Field label="Instructions" htmlFor={`sk-body-${i}`} hint="Markdown. Up to 64 KB.">
                      <TextArea id={`sk-body-${i}`} mono rows={14} value={k.body} onChange={(e) => set(skills.map((s, j) => (j === i ? { ...s, body: e.target.value } : s)))} />
                    </Field>
                  </div>
                )}
              </li>
            );
          })}
        </ul>
        <form
          className="flex flex-wrap items-center gap-2 border-t border-border-subtle px-[18px] py-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (!nameOk) return;
            set([...skills, { name, description: "", body: "" }]);
            setOpen(name);
            setName("");
          }}
        >
          <TextInput mono aria-label="New skill name" placeholder="skill-name" className="w-56" value={name} onChange={(e) => setName(e.target.value.toLowerCase())} />
          <Button type="submit" variant="outline" icon="plus" disabled={!nameOk}>
            Add skill
          </Button>
          {name && !nameOk && <span className="text-xs text-fg-tertiary">Lowercase letters, digits and -; must be unique.</span>}
        </form>
      </Card>
      <p className="text-xs text-fg-tertiary">
        Click a skill to edit its description and instructions (agent skills only). Project additions and removals belong to that project&apos;s override. The agent sees each
        description and loads the full playbook only when a task matches; on Claude Code they are native skills, and Codex and OpenCode read the same playbooks via{" "}
        <code className="inline">AGENTS.md</code>.
      </p>
    </div>
  );
}
