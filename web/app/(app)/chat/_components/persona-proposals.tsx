"use client";

import { ApiError, type Schemas } from "@/lib/api/client";
import { useDecidePersonaProposal, usePersonaProposals } from "@/lib/api/hooks";
import { Button } from "@/components/ui/button";
import { PersonaAvatar, PersonaName, personaLook, toneLabel } from "./persona";

type Proposal = Schemas["PersonaProposal"];

/**
 * Personas a persona proposed in this chat, waiting for the person: nothing is
 * created until they approve. Decided ones show as system messages instead.
 */
export function PersonaProposals({ chatId, disabled }: { chatId: string; disabled?: boolean }) {
  const proposals = usePersonaProposals(chatId);
  const decide = useDecidePersonaProposal(chatId);
  const pending = (proposals.data?.proposals ?? []).filter((p) => p.state === "pending");
  if (!pending.length) return null;
  const error = decide.error instanceof ApiError || decide.error instanceof Error ? decide.error.message : null;
  return (
    <li className="flex flex-col gap-2" data-testid="persona-proposals">
      {pending.map((p) => (
        <ProposalCard key={p.id} p={p} busy={decide.isPending || !!disabled} onDecide={(decision) => decide.mutate({ id: p.id, decision })} />
      ))}
      {error && (
        <p role="alert" className="text-[12.5px] text-pill-blocked-fg">
          {error}
        </p>
      )}
    </li>
  );
}

function ProposalCard({ p, busy, onDecide }: { p: Proposal; busy: boolean; onDecide: (d: "approve" | "decline") => void }) {
  const look = personaLook({ id: p.id, name: p.name, color: p.color });
  const by = personaLook(p.proposed_by);
  return (
    <div className="flex max-w-[min(720px,100%)] flex-col gap-2 rounded-xl border border-border bg-surface px-3.5 py-3" data-testid="persona-proposal">
      <p className="flex flex-wrap items-center gap-1.5 text-xs text-fg-tertiary">
        <PersonaAvatar persona={by} size={16} />
        <PersonaName persona={by} />
        <span>wants to create a new persona</span>
      </p>
      <div className="flex items-start gap-3">
        <PersonaAvatar persona={look} size={36} />
        <div className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span className="flex items-center gap-2">
            <PersonaName persona={look} className="text-[14px]" />
            <span className="rounded-full bg-selected px-2 py-0.5 text-[11px] text-fg-secondary">{toneLabel(p.tone)}</span>
          </span>
          {p.reason && <p className="text-[13px] text-fg-secondary">{p.reason}</p>}
          {p.custom_instructions && <p className="whitespace-pre-wrap text-[12.5px] text-fg-tertiary">{p.custom_instructions}</p>}
          <p className="text-[11.5px] text-fg-tertiary">Starts with no abilities (code, images, creating personas). You can change it later in Manage personas.</p>
        </div>
      </div>
      <div className="flex justify-end gap-2">
        <Button variant="outline" disabled={busy} onClick={() => onDecide("decline")}>
          Decline
        </Button>
        <Button variant="primary" disabled={busy} onClick={() => onDecide("approve")}>
          Approve {p.name}
        </Button>
      </div>
    </div>
  );
}
