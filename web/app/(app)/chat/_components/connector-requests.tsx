"use client";

import { useState } from "react";
import { ApiError, type Schemas } from "@/lib/api/client";
import { useConnectorApprovals, useConnectorProposals, useDecideConnectorApproval, useDeclineConnectorProposal } from "@/lib/api/connector-hooks";
import { Button } from "@/components/ui/button";
import { AddConnectorDialog } from "../../customization/connectors-section";
import { PersonaAvatar, PersonaName, personaLook } from "./persona";

const message = (e: unknown) => (e instanceof ApiError || e instanceof Error ? e.message : null);

/**
 * Tool calls waiting for a yes or no. Shown inside the running status card, so a request
 * and the step that needs it read as one thing instead of two stacked blocks.
 */
export function ConnectorApprovalCards({ chatId, disabled }: { chatId: string; disabled?: boolean }) {
  const approvals = useConnectorApprovals(chatId);
  const decide = useDecideConnectorApproval(chatId);
  const waiting = approvals.data?.approvals ?? [];
  if (!waiting.length) return null;
  const busy = decide.isPending || !!disabled;
  const error = message(decide.error);
  return (
    <>
      {waiting.map((a) => {
        const by = personaLook(a.persona);
        return (
          <div key={a.id} data-testid="connector-approval" className="flex flex-col gap-2 rounded-lg border border-pill-human-fg/35 bg-pill-human-bg px-3 py-2.5">
            <p className="flex flex-wrap items-center gap-1.5 text-13 text-fg">
              <PersonaAvatar persona={by} size={16} />
              <PersonaName persona={by} />
              <span>wants to use {a.connector_name}</span>
            </p>
            <div className="rounded-md border border-border-subtle bg-surface-sunken px-3 py-2">
              <p className="font-mono text-13 text-fg-2">{a.tool}</p>
              {Object.keys(a.arguments).length > 0 && <pre className="mt-1 max-h-40 overflow-auto whitespace-pre-wrap break-words text-12 text-fg-tertiary">{JSON.stringify(a.arguments, null, 2)}</pre>}
            </div>
            <div className="flex gap-2">
              <Button variant="primary" disabled={busy} onClick={() => decide.mutate({ id: a.id, decision: "allow" })}>Allow once</Button>
              <Button variant="outline" disabled={busy} onClick={() => decide.mutate({ id: a.id, decision: "always" })}>Always allow</Button>
              <Button variant="outline" disabled={busy} onClick={() => decide.mutate({ id: a.id, decision: "deny" })}>Deny</Button>
            </div>
          </div>
        );
      })}
      {error && <p role="alert" className="text-13 text-pill-blocked-fg">{error}</p>}
    </>
  );
}

/** Connectors a persona proposed in this chat. Tool approvals live in the status card ({@link ConnectorApprovalCards}). */
export function ConnectorRequests({ chatId, disabled }: { chatId: string; disabled?: boolean }) {
  const proposals = useConnectorProposals(chatId);
  const decline = useDeclineConnectorProposal();
  const [reviewing, setReviewing] = useState<Schemas["ConnectorProposal"] | null>(null);
  const proposed = proposals.data?.proposals ?? [];
  if (!proposed.length) return null;
  const error = message(decline.error);
  return (
    <li className="flex flex-col gap-2" data-testid="connector-requests">
      {proposed.map((p) => {
        const by = personaLook(p.proposed_by);
        return (
          <div key={p.id} data-testid="connector-proposal" className="flex max-w-[min(720px,100%)] flex-col gap-2 rounded-xl border border-border bg-surface px-3.5 py-3">
            <p className="flex flex-wrap items-center gap-1.5 text-12 text-fg-tertiary">
              <PersonaAvatar persona={by} size={16} />
              <PersonaName persona={by} />
              <span>wants to add a connector</span>
            </p>
            <p className="text-14 font-medium text-fg">{p.name}</p>
            <p className="break-all font-mono text-13 text-fg-secondary">{p.url}</p>
            {p.reason && <p className="text-13 text-fg-secondary">{p.reason}</p>}
            <div className="flex justify-end gap-2">
              <Button variant="outline" disabled={decline.isPending || !!disabled} onClick={() => decline.mutate(p.id)}>Decline</Button>
              <Button variant="primary" disabled={!!disabled} onClick={() => setReviewing(p)}>Review and connect</Button>
            </div>
          </div>
        );
      })}
      {error && <p role="alert" className="text-13 text-pill-blocked-fg">{error}</p>}
      {reviewing && <AddConnectorDialog proposal={reviewing} onClose={() => setReviewing(null)} />}
    </li>
  );
}
