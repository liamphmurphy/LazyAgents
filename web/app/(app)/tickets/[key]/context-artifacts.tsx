"use client";

import type { Ticket } from "@/lib/api/client";
import { useAttachArtifact, useDetachArtifact } from "@/lib/api/hooks";
import { Button } from "@/components/ui/button";
import { ArtifactPicker, DocCard } from "@/components/domain/artifacts";

/** "Context artifacts" on a ticket (ticket-detail.html): doc cards plus Attach. */
export function ContextArtifacts({ t }: { t: Ticket }) {
  const attach = useAttachArtifact(t.key);
  const detach = useDetachArtifact(t.key);
  return (
    <section className="flex flex-col gap-3">
      <div className="flex items-center justify-between gap-3">
        <h2 className="type-heading">Context artifacts</h2>
        <ArtifactPicker exclude={t.artifacts.map((a) => a.id)} onPick={(a) => attach.mutate(a.id)}>
          <Button variant="ghost" icon="plus" className="h-7 px-2 text-[12.5px] text-fg-secondary">
            Attach
          </Button>
        </ArtifactPicker>
      </div>
      {t.artifacts.length > 0 ? (
        <div className="grid grid-cols-[repeat(auto-fill,minmax(200px,1fr))] gap-2.5">
          {t.artifacts.map((d) => (
            <DocCard key={d.id} doc={d} assignee={t.assignee?.name} onRemove={d.relation === "context" ? () => detach.mutate(d.id) : undefined} />
          ))}
        </div>
      ) : (
        <p className="text-[13px] text-fg-tertiary">Attach the PRDs, specs and ADRs an agent should read before it starts.</p>
      )}
      {(attach.isError || detach.isError) && <p className="text-xs text-pill-blocked-fg">{(attach.error ?? detach.error)?.message}</p>}
    </section>
  );
}
