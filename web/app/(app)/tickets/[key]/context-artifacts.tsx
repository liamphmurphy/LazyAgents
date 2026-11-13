"use client";

import Link from "next/link";
import { useState } from "react";
import type { Ticket } from "@/lib/api/client";
import { useAttachArtifact, useDetachArtifact } from "@/lib/api/hooks";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { Banner } from "@/components/ui/primitives";
import { NewMockupDialog } from "@/components/domain/artifact-dialogs";
import { ReviewChip } from "@/components/domain/mockup";
import { ArtifactPicker, DocCard } from "@/components/domain/artifacts";

/** Mockup requirement: approved (done) or the list of mockups still needing a person. */
function MockupGate({ t, onNew }: { t: Ticket; onNew: () => void }) {
  const g = t.mockup_gate;
  if (!g.required) return null;
  const list = (
    <ul className="mt-1 flex flex-col gap-1">
      {g.mockups.map((m) => (
        <li key={m.id} className="flex flex-wrap items-center gap-2 text-13">
          <Link href={`/artifacts/${m.id}`} className="font-medium text-fg hover:underline">
            {m.title}
          </Link>
          <span className="font-mono text-12 text-fg-tertiary">{m.version_label}</span>
          <ReviewChip state={m.review_state} />
          <Link href={`/artifacts/${m.id}/review?ticket=${encodeURIComponent(t.key)}`} className="text-12 text-accent hover:underline">
            {m.review_state === "pending" ? "Review" : "Open review"}
          </Link>
          {m.review_state === "changes_requested" && m.review_note && <span className="basis-full whitespace-pre-line text-12 text-fg-secondary">“{m.review_note}”</span>}
        </li>
      ))}
    </ul>
  );
  if (g.approved) {
    return (
      <div className="flex flex-col gap-0.5 rounded-xl border border-pill-done-bg bg-pill-done-bg/40 px-4 py-3">
        <span className="flex items-center gap-1.5 font-semibold text-pill-done-fg">
          <Icon name="check" size={14} /> Mockup approved
        </span>
        {list}
      </div>
    );
  }
  return (
    <Banner
      tone="human"
      title="Needs an approved UI mockup — the agent can't finish until one is approved"
      actions={
        <Button variant="approve-outline" icon="plus" onClick={onNew}>
          New mockup
        </Button>
      }
    >
      {g.mockups.length ? list : "No mockup yet. Create one, or have the agent draft it."}
    </Banner>
  );
}

/** "Context artifacts" on a ticket (ticket-detail.html): doc cards plus Attach. */
export function ContextArtifacts({ t }: { t: Ticket }) {
  const attach = useAttachArtifact(t.key);
  const detach = useDetachArtifact(t.key);
  const [mockup, setMockup] = useState(false);
  return (
    <section className="flex flex-col gap-3">
      <div className="flex items-center justify-between gap-3">
        <h2 className="type-heading">Context artifacts</h2>
        <div className="flex items-center gap-1">
          <Button variant="ghost" icon="plus" className="h-7 px-2 text-13 text-fg-secondary" onClick={() => setMockup(true)}>
            New mockup
          </Button>
          <ArtifactPicker exclude={t.artifacts.map((a) => a.id)} onPick={(a) => attach.mutate(a.id)}>
            <Button variant="ghost" icon="plus" className="h-7 px-2 text-13 text-fg-secondary">
              Attach
            </Button>
          </ArtifactPicker>
        </div>
      </div>
      <MockupGate t={t} onNew={() => setMockup(true)} />
      {t.artifacts.length > 0 ? (
        <div className="grid grid-cols-[repeat(auto-fill,minmax(200px,1fr))] gap-2.5">
          {t.artifacts.map((d) => (
            <DocCard key={d.id} doc={d} assignee={t.assignee?.name} onRemove={d.relation === "context" ? () => detach.mutate(d.id) : undefined} />
          ))}
        </div>
      ) : (
        <p className="text-13 text-fg-tertiary">Attach the PRDs, specs and ADRs an agent should read before it starts.</p>
      )}
      {(attach.isError || detach.isError) && <p className="text-12 text-pill-blocked-fg">{(attach.error ?? detach.error)?.message}</p>}
      {mockup && <NewMockupDialog open ticketKey={t.key} defaultProject={t.project.key} defaultEpicId={t.epic?.id} onOpenChange={setMockup} />}
    </section>
  );
}
