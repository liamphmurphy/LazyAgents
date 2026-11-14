"use client";

import Link from "next/link";
import { DropdownMenu, Popover } from "radix-ui";
import { useState, type ReactNode } from "react";
import type { Schemas, TicketPatch } from "@/lib/api/client";
import { cx } from "@/lib/cx";
import { useBulkUpdate } from "@/lib/api/hooks";
import { ESTIMATE_SCALE } from "@/lib/ticket-draft";
import { failureReason, type BulkResult } from "@/lib/ticket-list";
import { Button, IconButton } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { ArtifactPicker } from "@/components/domain/artifacts";
import { DispatchDialog } from "@/components/domain/dispatch-dialog";
import { PRIORITY_NAME, STATUS_NAME } from "@/components/domain/meta";
import { AgentPicker, EpicMenu, MockupRequirementMenu, PriorityMenu, StatusMenu, floatPanel } from "@/components/domain/pickers";

const item =
  "flex h-8 cursor-default select-none items-center gap-2.5 rounded-md px-2 outline-none data-[highlighted]:bg-hover data-[disabled]:opacity-50";

/** 1·2·3·5·8·13 picker plus Clear, same scale as the ticket's estimate. */
function EstimateMenu({ onChange, children }: { onChange: (points: number | null) => void; children: ReactNode }) {
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>{children}</DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align="end" sideOffset={6} className={cx(floatPanel, "w-[216px] p-2")}>
          <DropdownMenu.Label className="px-1 pb-1.5 text-12 text-fg-tertiary">Set estimate</DropdownMenu.Label>
          <div className="flex gap-1">
            {ESTIMATE_SCALE.map((p) => (
              <DropdownMenu.Item
                key={p}
                onSelect={() => onChange(p)}
                aria-label={`${p} points`}
                className="grid h-[26px] min-w-7 flex-1 cursor-default place-items-center rounded-[5px] border border-border-strong font-mono text-12 outline-none data-[highlighted]:border-accent data-[highlighted]:bg-accent/18"
              >
                {p}
              </DropdownMenu.Item>
            ))}
          </div>
          <DropdownMenu.Separator className="my-1.5 h-px bg-border-subtle" />
          <DropdownMenu.Item className={cx(item, "text-fg-secondary")} onSelect={() => onChange(null)}>
            Clear estimate
          </DropdownMenu.Item>
          <p className="px-1 pt-1 text-11 text-fg-tertiary">Records you as the estimator.</p>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}

/**
 * Floating bulk-action bar (DESIGN.md §4). Status, assignee, epic, priority
 * and estimate are saved ticket by ticket, so one refusal (a 409 for a live
 * run) doesn't undo the rest; onResult reports what happened.
 */
export function BulkBar({
  keys,
  agents,
  epics,
  onClear,
  onApply,
  saving,
}: {
  keys: string[];
  agents: Schemas["Agent"][];
  epics: Schemas["Epic"][];
  onClear: () => void;
  onApply: (patch: TicketPatch, label: string) => void;
  saving: boolean;
}) {
  const bulk = useBulkUpdate();
  const [dispatching, setDispatching] = useState(false);
  const [more, setMore] = useState(false);
  const busy = saving || bulk.isPending;

  const apply = onApply;
  const agentName = (id: string | null) => (id ? agents.find((a) => a.id === id)?.name ?? "persona" : "Unassigned");
  const epicName = (id: string | null) => (id ? epics.find((e) => e.id === id)?.name ?? "epic" : "No epic");

  return (
    <div
      role="toolbar"
      aria-label="Bulk actions"
      className="fixed bottom-6 left-1/2 z-40 flex w-[min(760px,calc(100%-32px))] -translate-x-1/2 flex-wrap items-center gap-1 rounded-xl border border-border-float bg-surface-raised py-2 pl-4 pr-2 shadow-[var(--shadow-float)]"
    >
      <span className="mr-auto font-medium" aria-live="polite">
        {keys.length} selected{busy && <span className="ml-2 text-fg-tertiary">Saving…</span>}
        {bulk.isError && <span className="ml-2 text-pill-blocked-fg">{bulk.error.message}</span>}
      </span>
      <StatusMenu onChange={(status) => apply({ status }, `Status → ${STATUS_NAME[status]}`)} align="end">
        <Button variant="ghost" disabled={busy}>Status</Button>
      </StatusMenu>
      <AgentPicker label="Assignee" agents={agents.filter((a) => a.kind === "main")} onChange={(id) => apply({ assignee_agent_id: id }, `Assignee → ${agentName(id)}`)}>
        <Button variant="ghost" disabled={busy}>Assignee</Button>
      </AgentPicker>
      <EpicMenu epics={epics} onChange={(id) => apply({ epic_id: id }, `Epic → ${epicName(id)}`)}>
        <Button variant="ghost" disabled={busy}>Epic</Button>
      </EpicMenu>
      <PriorityMenu onChange={(priority) => apply({ priority }, `Priority → ${PRIORITY_NAME[priority]}`)} align="end">
        <Button variant="ghost" disabled={busy}>Priority</Button>
      </PriorityMenu>
      <EstimateMenu onChange={(p) => apply({ estimate_points: p }, p == null ? "Estimate cleared" : `Estimate → ${p}`)}>
        <Button variant="ghost" disabled={busy}>Estimate</Button>
      </EstimateMenu>
      <span aria-hidden className="mx-1 h-6 w-px bg-border-float" />
      <Button variant="approve-outline" onClick={() => setDispatching(true)}>
        Dispatch…
      </Button>
      <Popover.Root open={more} onOpenChange={setMore}>
        <Popover.Trigger asChild>
          <IconButton icon="dots" label="More bulk actions" />
        </Popover.Trigger>
        <Popover.Portal>
          <Popover.Content align="end" sideOffset={6} className={cx(floatPanel, "flex w-48 flex-col")}>
            <MockupRequirementMenu onChange={(requires_mockup) => { setMore(false); bulk.mutate({ keys, patch: { requires_mockup } }); }} align="end">
              <button type="button" className={cx(item, "w-full hover:bg-hover")}>Mockup…</button>
            </MockupRequirementMenu>
            <ArtifactPicker onPick={(a) => { setMore(false); bulk.mutate({ keys, patch: {}, attach_artifact_id: a.id }); }}>
              <button type="button" className={cx(item, "w-full hover:bg-hover")}>Attach artifact…</button>
            </ArtifactPicker>
          </Popover.Content>
        </Popover.Portal>
      </Popover.Root>
      <IconButton icon="x" label="Clear selection" onClick={onClear} />
      {dispatching && <DispatchDialog keys={keys} open onOpenChange={setDispatching} onDone={onClear} />}
    </div>
  );
}

/** Outcome of the last bulk action: per-ticket failures with HTTP status and reason. */
export function BulkResultPanel({
  result,
  onSelectFailed,
  onDismiss,
  raised,
}: {
  result: BulkResult;
  onSelectFailed: () => void;
  onDismiss: () => void;
  /** Sit above the bulk bar while it's showing. */
  raised: boolean;
}) {
  const { label, updated, failed } = result;
  const ok = failed.length === 0;
  const updatedText =
    updated.length === 0 ? "" : updated.length <= 3 ? `${updated.join(", ")} updated.` : `${updated.length} tickets updated.`;
  return (
    <div
      role={ok ? "status" : "alert"}
      aria-label="Bulk result"
      className={cx(
        "fixed left-1/2 z-40 w-[min(760px,calc(100%-32px))] -translate-x-1/2 rounded-xl border border-border-float bg-surface-raised shadow-[var(--shadow-float)]",
        raised ? "bottom-[88px]" : "bottom-6",
      )}
    >
      <div className="flex items-center gap-2 px-4 py-2.5">
        {ok ? (
          <Icon name="check" size={14} className="text-state-done" />
        ) : (
          <span aria-hidden className="grid size-3.5 place-items-center rounded-full border border-state-human bg-pill-human-bg text-10 font-bold text-pill-human-fg">!</span>
        )}
        <span className="font-medium">{label}</span>
        <span className="text-fg-secondary">
          · {updated.length} updated{failed.length > 0 && `, ${failed.length} failed`}
        </span>
        <span className="ml-auto flex items-center gap-1">
          {!ok && (
            <Button variant="outline" className="h-6 px-2 text-12" onClick={onSelectFailed}>
              Select failed
            </Button>
          )}
          <IconButton icon="x" label="Dismiss" onClick={onDismiss} />
        </span>
      </div>
      {!ok && (
        <ul className="max-h-48 overflow-y-auto border-t border-border-subtle px-4 py-2">
          {failed.map((f) => (
            <li key={f.key} className="flex h-8 items-center gap-2.5">
              <span className="font-mono text-12 text-fg-secondary">{f.key}</span>
              <span className="rounded-[4px] bg-pill-blocked-bg px-1.5 font-mono text-12 text-pill-blocked-fg">{f.status || "error"}</span>
              <span className="min-w-0 flex-1 truncate">{failureReason(f)}</span>
              <Link href={`/tickets/${f.key}`} className="flex-none text-accent hover:underline">
                Open ticket →
              </Link>
            </li>
          ))}
          {updatedText && (
            <li className="flex h-7 items-center gap-2 text-13 text-fg-secondary">
              <Icon name="check" size={13} className="text-state-done" />
              {updatedText}
            </li>
          )}
        </ul>
      )}
    </div>
  );
}
