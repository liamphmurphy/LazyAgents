"use client";

import { useState } from "react";
import { ApiError, type Schemas } from "@/lib/api/client";
import { useApiSecretApprovals, useApiSecretProposals, useDecideApiSecretApproval, useDeclineApiSecretProposal } from "@/lib/api/secret-hooks";
import { placementLabel, sentAs } from "@/lib/secrets-ui";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { AddSecretDialog } from "../../customization/api-secrets-section";
import { PersonaAvatar, PersonaName, personaLook } from "./persona";

const message = (e: unknown) => (e instanceof ApiError || e instanceof Error ? e.message : null);

function SecretChip({ name }: { name: string }) {
  return (
    <span className="inline-flex items-center gap-1 rounded-full bg-pill-human-bg px-2 py-0.5 text-[12px] text-pill-human-fg">
      <Icon name="key" size={12} />
      {name}
    </span>
  );
}

/** Requests with a secret waiting for a yes or no, and secrets a persona asked for in this chat. */
export function SecretRequests({ chatId, disabled }: { chatId: string; disabled?: boolean }) {
  const approvals = useApiSecretApprovals(chatId);
  const proposals = useApiSecretProposals(chatId);
  const decide = useDecideApiSecretApproval(chatId);
  const decline = useDeclineApiSecretProposal();
  const [reviewing, setReviewing] = useState<Schemas["ApiSecretProposal"] | null>(null);
  const waiting = approvals.data?.approvals ?? [];
  const proposed = proposals.data?.proposals ?? [];
  if (!waiting.length && !proposed.length) return null;
  const busy = decide.isPending || !!disabled;
  const error = message(decide.error) ?? message(decline.error);
  return (
    <li className="flex flex-col gap-2" data-testid="secret-requests">
      {waiting.map((a) => {
        const by = personaLook(a.persona);
        const read = a.method === "GET" || a.method === "HEAD";
        let host = a.url;
        try {
          host = new URL(a.url).hostname;
        } catch {}
        return (
          <div key={a.id} data-testid="secret-approval" className="flex max-w-[min(720px,100%)] flex-col gap-2.5 rounded-xl border border-pill-human-fg/40 bg-surface px-3.5 py-3">
            <p className="flex flex-wrap items-center gap-1.5 text-[13.5px] text-fg">
              <PersonaAvatar persona={by} size={18} />
              <PersonaName persona={by} />
              <span>wants to call an API with</span>
              <SecretChip name={a.secret_name} />
            </p>
            <div className="rounded-md border border-border-subtle bg-surface-sunken px-3 py-2 font-mono text-[12.5px] text-fg-2">
              <p className="break-all"><span className="font-semibold">{a.method}</span> {a.url}</p>
              <p className="mt-1 flex flex-wrap items-center gap-1.5">
                <span className="rounded bg-pill-human-bg px-1.5 text-pill-human-fg">{sentAs(a.placement, a.placement_key, a.secret_handle)}</span>
                <span className="font-sans text-xs text-fg-tertiary">added by Vesper when sent</span>
              </p>
              {Object.entries(a.headers).map(([k, v]) => <p key={k} className="break-all text-fg-secondary">{k}: {v}</p>)}
              {a.body && (
                <>
                  <p className="mt-1.5 font-sans text-xs text-fg-tertiary">Body · {new TextEncoder().encode(a.body).length.toLocaleString()} bytes, all of it shown</p>
                  <pre data-testid="secret-approval-body" className="max-h-60 overflow-auto whitespace-pre-wrap break-words text-[12px] text-fg-secondary">{a.body}</pre>
                </>
              )}
            </div>
            <p className="flex flex-wrap gap-x-4 gap-y-1 text-[12px] text-fg-secondary">
              <span className="flex items-center gap-1"><Icon name="check" size={12} className="text-pill-done-fg" /> {host} is an allowed host</span>
              <span>{read ? "A read" : "A write"} · set to Ask</span>
            </p>
            <div className="flex flex-wrap gap-2">
              <Button variant="primary" disabled={busy} onClick={() => decide.mutate({ id: a.id, decision: "allow" })}>Allow once</Button>
              {read && <Button variant="outline" disabled={busy} onClick={() => decide.mutate({ id: a.id, decision: "always" })}>Always allow reads</Button>}
              <Button variant="outline" disabled={busy} onClick={() => decide.mutate({ id: a.id, decision: "deny" })}>Deny</Button>
            </div>
            <p className="text-xs text-fg-tertiary">{by?.name ?? "The persona"} never gets the value. Copies of it in the response are masked as ‹{a.secret_handle}›, as far as Vesper can recognise them.</p>
          </div>
        );
      })}
      {proposed.map((p) => {
        const by = personaLook(p.proposed_by);
        return (
          <div key={p.id} data-testid="secret-proposal" className="flex max-w-[min(720px,100%)] flex-col gap-2 rounded-xl border border-border bg-surface px-3.5 py-3">
            <p className="flex flex-wrap items-center gap-1.5 text-xs text-fg-tertiary">
              <Icon name="key" size={13} className="text-pill-human-fg" />
              <PersonaAvatar persona={by} size={16} />
              <PersonaName persona={by} />
              <span>asks for a secret</span>
            </p>
            <p className="text-[14px] font-medium text-fg">{p.name}</p>
            <p className="text-[12.5px] text-fg-secondary">
              Sent only to <span data-testid="secret-proposal-host" className="break-all rounded bg-surface-sunken px-1.5 py-0.5 font-mono text-[13px] font-semibold text-fg">{p.host}</span> · {placementLabel(p.placement, p.placement_key)}
            </p>
            <p className="text-xs text-fg-tertiary">Check the host is the API you expect: the name is the persona’s suggestion.</p>
            {p.reason && <p className="text-[13px] text-fg-secondary">{p.reason}</p>}
            <p className="text-xs text-fg-tertiary">You enter the value in Vesper, never in the chat.</p>
            <div className="flex justify-end gap-2">
              <Button variant="outline" disabled={decline.isPending || !!disabled} onClick={() => decline.mutate(p.id)}>Decline</Button>
              <Button variant="primary" disabled={!!disabled} onClick={() => setReviewing(p)}>Add secret…</Button>
            </div>
          </div>
        );
      })}
      {error && <p role="alert" className="text-[12.5px] text-pill-blocked-fg">{error}</p>}
      {reviewing && <AddSecretDialog proposal={reviewing} onClose={() => setReviewing(null)} />}
    </li>
  );
}

