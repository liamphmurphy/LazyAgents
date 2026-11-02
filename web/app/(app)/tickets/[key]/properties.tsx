"use client";

import { useState, type ComponentProps, type ReactNode } from "react";
import type { Bootstrap, Ticket } from "@/lib/api/client";
import { useUpdateTicket } from "@/lib/api/hooks";
import { cx } from "@/lib/cx";
import { absoluteTime, ago, dueDate } from "@/lib/format";
import { Icon } from "@/components/ui/icon";
import { AgentAvatar, HumanAvatar, UnassignedAvatar } from "@/components/domain/avatars";
import { PriorityGlyph, StatusGlyph } from "@/components/domain/glyphs";
import { Flag, LabelChip } from "@/components/domain/labels";
import { needsDispatch } from "./run-parts";
import { PRIORITY_NAME, STATUS_NAME } from "@/components/domain/meta";
import { AgentPicker, EpicMenu, LabelPicker, PriorityMenu, StatusMenu, UserMenu } from "@/components/domain/pickers";

/**
 * Property row as a button (DESIGN.md §4): the whole row is the hit target.
 * Spreads props so Radix triggers (asChild) can attach handlers and refs.
 */
function PropButton({ label, children, className, ...rest }: { label: string; children: ReactNode } & ComponentProps<"button">) {
  return (
    <button
      type="button"
      className={cx("flex min-h-[34px] w-full items-center gap-2.5 rounded-md px-2 text-left hover:bg-hover", className)}
      {...rest}
    >
      <span className="w-[88px] flex-none text-[12.5px] text-fg-tertiary">{label}</span>
      <span className="flex min-w-0 flex-1 flex-wrap items-center gap-2 text-[13px] text-fg">{children}</span>
    </button>
  );
}

function Empty({ children }: { children: ReactNode }) {
  return <span className="text-fg-tertiary">{children}</span>;
}

/** Inline editor for short free-text and date properties. */
function InlineProp({
  label,
  value,
  display,
  type = "text",
  placeholder,
  onSave,
}: {
  label: string;
  value: string;
  display: ReactNode;
  type?: "text" | "date";
  placeholder: string;
  onSave: (v: string | null) => void;
}) {
  const [editing, setEditing] = useState(false);
  if (!editing) {
    return (
      <PropButton label={label} onClick={() => setEditing(true)}>
        {display}
      </PropButton>
    );
  }
  const commit = (v: string) => {
    setEditing(false);
    if (v !== value) onSave(v.trim() ? v.trim() : null);
  };
  return (
    <div className="flex min-h-[34px] items-center gap-2.5 px-2">
      <label htmlFor={`prop-${label}`} className="w-[88px] flex-none text-[12.5px] text-fg-tertiary">
        {label}
      </label>
      <input
        id={`prop-${label}`}
        type={type}
        autoFocus
        defaultValue={value}
        placeholder={placeholder}
        onBlur={(e) => commit(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") commit(e.currentTarget.value);
          if (e.key === "Escape") setEditing(false);
        }}
        className="h-7 min-w-0 flex-1 rounded-md border border-border-strong bg-surface px-2 text-[13px] text-fg outline-none focus:border-accent [color-scheme:inherit]"
      />
    </div>
  );
}

export function Properties({ t, boot }: { t: Ticket; boot: Bootstrap }) {
  const update = useUpdateTicket(t.key);
  const epics = boot.epics.filter((e) => e.project_id === t.project.id);
  const reviewer = boot.users.find((u) => u.id === t.reviewer?.id);

  return (
    <div className="flex flex-col gap-px">
      <StatusMenu value={t.status} onChange={(status) => update.mutate({ status })}>
        <PropButton label="Status">
          <StatusGlyph state={t.status} surface="var(--bg-panel)" label={false} />
          {STATUS_NAME[t.status]}
        </PropButton>
      </StatusMenu>
      <PriorityMenu value={t.priority} onChange={(priority) => update.mutate({ priority })}>
        <PropButton label="Priority">
          <PriorityGlyph priority={t.priority} label={false} />
          {PRIORITY_NAME[t.priority]}
        </PropButton>
      </PriorityMenu>
      <AgentPicker agents={boot.agents} value={t.assignee?.id} onChange={(id) => update.mutate({ assignee_agent_id: id })}>
        <PropButton label="Agent">
          {t.assignee ? (
            <>
              <AgentAvatar name={t.assignee.name} colorIndex={t.assignee.color_index} label={false} />
              {t.assignee.name}
              {needsDispatch(t) ? (
                <Flag tone="human">Not dispatched</Flag>
              ) : t.run?.state === "queued" ? (
                <Flag tone="neutral">Queued</Flag>
              ) : null}
            </>
          ) : (
            <>
              <UnassignedAvatar />
              <Empty>Unassigned</Empty>
            </>
          )}
        </PropButton>
      </AgentPicker>
      <UserMenu users={boot.users} value={t.reviewer?.id ?? null} onChange={(id) => update.mutate({ reviewer_user_id: id })}>
        <PropButton label="Reviewer">
          {t.reviewer ? (
            <>
              <HumanAvatar name={t.reviewer.name} initials={t.reviewer.initials} color={reviewer?.avatar_color} label={false} />
              {t.reviewer.name}
            </>
          ) : (
            <Empty>No reviewer</Empty>
          )}
        </PropButton>
      </UserMenu>
      <EpicMenu epics={epics} value={t.epic?.id ?? null} onChange={(id) => update.mutate({ epic_id: id })}>
        <PropButton label="Epic">
          {t.epic ? (
            <>
              <Icon name="epics" size={14} className="text-fg-tertiary" />
              {t.epic.name}
            </>
          ) : (
            <Empty>No epic</Empty>
          )}
        </PropButton>
      </EpicMenu>
      <div className="flex min-h-[34px] items-center gap-2.5 px-2">
        <span className="w-[88px] flex-none text-[12.5px] text-fg-tertiary">Project</span>
        <span className="flex items-center gap-2 text-[13px]">
          <span aria-hidden className="size-2.5 rounded-[3px]" style={{ background: t.project.color }} />
          {t.project.name}
        </span>
      </div>
      <InlineProp
        label="Estimate"
        value={t.estimate ?? ""}
        placeholder="e.g. M · ~3h agent time"
        display={t.estimate ?? <Empty>None</Empty>}
        onSave={(v) => update.mutate({ estimate: v })}
      />
      <InlineProp
        label="Due"
        type="date"
        value={t.due_date ?? ""}
        placeholder=""
        display={t.due_date ? dueDate(t.due_date) : <Empty>No due date</Empty>}
        onSave={(v) => update.mutate({ due_date: v })}
      />
      <LabelPicker labels={boot.labels} value={t.labels.map((l) => l.id)} onChange={(ids) => update.mutate({ label_ids: ids })}>
        <PropButton label="Labels" className="py-1.5">
          {t.labels.length ? t.labels.map((l) => <LabelChip key={l.id} name={l.name} color={l.color} />) : <Empty>Add labels</Empty>}
        </PropButton>
      </LabelPicker>
      {update.isError && (
        <p role="alert" className="px-2 pt-2 text-xs text-pill-blocked-fg">
          {update.error.message}
        </p>
      )}
    </div>
  );
}

/** Code section: branch and PR (set by agents in Phase 4, synced from GitHub in Phase 5). */
export function CodeSection({ t }: { t: Ticket }) {
  if (!t.branch && !t.pr) return null;
  const prState = t.pr ? t.pr.state[0].toUpperCase() + t.pr.state.slice(1) : "";
  return (
    <section className="flex flex-col gap-3 px-2">
      <h2 className="text-[12.5px] text-fg-tertiary">Code</h2>
      {t.branch && (
        <div className="flex items-center gap-2.5 font-mono text-xs text-fg-2">
          <Icon name="branch" size={14} className="text-fg-tertiary" />
          <span className="truncate">{t.branch}</span>
        </div>
      )}
      {t.pr && (
        <div className="flex items-center gap-2.5 text-[13px]">
          <Icon name="pr" size={14} className="text-fg-tertiary" />
          {t.pr.url ? (
            <a href={t.pr.url} target="_blank" rel="noreferrer noopener" className="hover:underline">
              #{t.pr.number} · {prState}
            </a>
          ) : (
            <span>
              #{t.pr.number} · {prState}
            </span>
          )}
          {t.pr.additions != null && (
            <span className="ml-auto font-mono text-xs">
              <span className="text-pill-done-fg">+{t.pr.additions}</span>{" "}
              <span className="text-pill-blocked-fg">−{t.pr.deletions ?? 0}</span>
            </span>
          )}
        </div>
      )}
      {t.pr?.closed_unmerged && <Flag tone="blocked">PR closed without merging</Flag>}
      {t.pr?.synced_at && (
        <span className="text-[11.5px] text-fg-tertiary" title={absoluteTime(t.pr.synced_at)}>
          Synced from GitHub {ago(t.pr.synced_at)}
        </span>
      )}
    </section>
  );
}
