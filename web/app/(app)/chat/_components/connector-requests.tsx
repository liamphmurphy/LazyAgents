"use client";

import { useState } from "react";
import { ApiError, type Schemas } from "@/lib/api/client";
import { useConnectorApprovals, useConnectorProposals, useDecideConnectorApproval, useDeclineConnectorProposal } from "@/lib/api/connector-hooks";
import { Button } from "@/components/ui/button";
import { AddConnectorDialog } from "../../customization/connectors-section";
import { PersonaAvatar, PersonaName, personaLook } from "./persona";

const message = (e: unknown) => (e instanceof ApiError || e instanceof Error ? e.message : null);

/** Tool calls waiting for a yes or no, and connectors a persona proposed in this chat. */
export function ConnectorRequests({ chatId, disabled }: { chatId: string; disabled?: boolean }) {
  const approvals = useConnectorApprovals(chatId);
  const proposals = useConnectorProposals(chatId);
  const decide = useDecideConnectorApproval(chatId);
  const decline = useDeclineConnectorProposal();
  const [reviewing, setReviewing] = useState<Schemas["ConnectorProposal"] | null>(null);
  const waiting = approvals.data?.approvals ?? [];
  const proposed = proposals.data?.proposals ?? [];
  if (!waiting.length && !proposed.length) return null;
  const busy = decide.isPending || !!disabled;
  const error = message(decide.error) ?? message(decline.error);
  return (
    <li className="flex flex-col gap-2" data-testid="connector-requests">
      {waiting.map((a) => {
        const by = personaLook(a.persona);
        return (
          <div key={a.id} data-testid="connector-approval" className="flex max-w-[min(720px,100%)] flex-col gap-2 rounded-xl border border-border bg-surface px-3.5 py-3">
            <p className="flex flex-wrap items-center gap-1.5 text-[13.5px] text-fg">
              <PersonaAvatar persona={by} size={18} />
              <PersonaName persona={by} />
              <span>wants to use {a.connector_name}</span>
            </p>
            <div className="rounded-md border border-border-subtle bg-surface-sunken px-3 py-2">
              <p className="font-mono text-[12.5px] text-fg-2">{a.tool}</p>
              {Object.keys(a.arguments).length > 0 && <pre className="mt-1 max-h-40 overflow-auto whitespace-pre-wrap break-words text-[12px] text-fg-tertiary">{JSON.stringify(a.arguments, null, 2)}</pre>}
            </div>
            <div className="flex gap-2">
              <Button variant="primary" disabled={busy} onClick={() => decide.mutate({ id: a.id, decision: "allow" })}>Allow once</Button>
              <Button variant="outline" disabled={busy} onClick={() => decide.mutate({ id: a.id, decision: "always" })}>Always allow</Button>
              <Button variant="outline" disabled={busy} onClick={() => decide.mutate({ id: a.id, decision: "deny" })}>Deny</Button>
            </div>
          </div>
        );
      })}
      {proposed.map((p) => {
        const by = personaLook(p.proposed_by);
        return (
          <div key={p.id} data-testid="connector-proposal" className="flex max-w-[min(720px,100%)] flex-col gap-2 rounded-xl border border-border bg-surface px-3.5 py-3">
            <p className="flex flex-wrap items-center gap-1.5 text-xs text-fg-tertiary">
              <PersonaAvatar persona={by} size={16} />
              <PersonaName persona={by} />
              <span>wants to add a connector</span>
            </p>
            <p className="text-[14px] font-medium text-fg">{p.name}</p>
            <p className="break-all font-mono text-[12.5px] text-fg-secondary">{p.url}</p>
            {p.reason && <p className="text-[13px] text-fg-secondary">{p.reason}</p>}
            <div className="flex justify-end gap-2">
              <Button variant="outline" disabled={decline.isPending || !!disabled} onClick={() => decline.mutate(p.id)}>Decline</Button>
              <Button variant="primary" disabled={!!disabled} onClick={() => setReviewing(p)}>Review and connect</Button>
            </div>
          </div>
        );
      })}
      {error && <p role="alert" className="text-[12.5px] text-pill-blocked-fg">{error}</p>}
      {reviewing && <AddConnectorDialog proposal={reviewing} onClose={() => setReviewing(null)} />}
    </li>
  );
}
