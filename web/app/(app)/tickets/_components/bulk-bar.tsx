"use client";

import { useState } from "react";
import type { Schemas } from "@/lib/api/client";
import { useBulkUpdate } from "@/lib/api/hooks";
import { Button, IconButton } from "@/components/ui/button";
import { ArtifactPicker } from "@/components/domain/artifacts";
import { DispatchDialog } from "@/components/domain/dispatch-dialog";
import { AgentPicker, PriorityMenu, StatusMenu } from "@/components/domain/pickers";

/** Floating bulk-action bar (DESIGN.md §4). */
export function BulkBar({ keys, agents, onClear }: { keys: string[]; agents: Schemas["Agent"][]; onClear: () => void }) {
  const bulk = useBulkUpdate();
  const apply = (patch: Schemas["TicketPatch"]) => bulk.mutate({ keys, patch });
  const [dispatching, setDispatching] = useState(false);
  return (
    <div
      role="toolbar"
      aria-label="Bulk actions"
      className="fixed bottom-6 left-1/2 z-40 flex w-[min(640px,calc(100%-32px))] -translate-x-1/2 flex-wrap items-center gap-1 rounded-xl border border-border-float bg-surface-raised py-2 pl-4 pr-2 shadow-[var(--shadow-float)]"
    >
      <span className="mr-auto font-medium" aria-live="polite">
        {keys.length} selected{bulk.isPending && <span className="ml-2 text-fg-tertiary">Saving…</span>}
        {bulk.isError && <span className="ml-2 text-pill-blocked-fg">{bulk.error.message}</span>}
      </span>
      <StatusMenu onChange={(status) => apply({ status })} align="end">
        <Button variant="ghost">Status</Button>
      </StatusMenu>
      <PriorityMenu onChange={(priority) => apply({ priority })} align="end">
        <Button variant="ghost">Priority</Button>
      </PriorityMenu>
      <ArtifactPicker onPick={(a) => bulk.mutate({ keys, patch: {}, attach_artifact_id: a.id })}>
        <Button variant="ghost">Attach artifact</Button>
      </ArtifactPicker>
      <AgentPicker agents={agents.filter((a) => a.kind === "main")} onChange={(id) => apply({ assignee_agent_id: id })}>
        <Button variant="secondary" icon="agents">
          Assign agent
        </Button>
      </AgentPicker>
      <Button variant="approve-outline" onClick={() => setDispatching(true)}>
        Dispatch…
      </Button>
      <IconButton icon="x" label="Clear selection" onClick={onClear} />
      {dispatching && <DispatchDialog keys={keys} open onOpenChange={setDispatching} onDone={onClear} />}
    </div>
  );
}
