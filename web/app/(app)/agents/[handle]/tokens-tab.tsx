"use client";

import { useState } from "react";
import type { AgentDetail } from "@/lib/api/client";
import { useAgentTokens, useCreateToken, useRevokeToken } from "@/lib/api/hooks";
import { cx } from "@/lib/cx";
import { ago } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { CopyButton } from "@/components/ui/copy-button";
import { Banner, Card, EmptyState, Field, Skeleton, TextInput } from "@/components/ui/primitives";

function Command({ label, text }: { label: string; text: string }) {
  return (
    <div className="flex flex-col gap-1">
      <span className="text-xs text-fg-tertiary">{label}</span>
      <div className="flex items-start gap-2 rounded-lg border border-border bg-surface-sunken p-2.5">
        <pre className="min-w-0 flex-1 overflow-x-auto whitespace-pre-wrap break-all font-mono text-xs text-fg-2">{text}</pre>
        <CopyButton text={text} label={`Copy ${label}`} />
      </div>
    </div>
  );
}

/**
 * Tokens for unmanaged sessions (PLAN.md §1): someone runs Claude Code or
 * Codex themselves and attaches it to this agent's identity. Vesper
 * can't enforce budgets or isolation on those, only brokered scopes.
 */
export function TokensTab({ detail }: { detail: AgentDetail }) {
  const handle = detail.agent.handle;
  const list = useAgentTokens(handle);
  const create = useCreateToken(handle);
  const revoke = useRevokeToken(handle);
  const [label, setLabel] = useState("");
  const minted = create.data;

  return (
    <div className="flex flex-col gap-4">
      <Banner tone="human" title="Unmanaged sessions">
        A token lets a Claude Code or Codex session you run yourself act as {detail.agent.name}: same instructions, same Agent API. Vesper
        can&apos;t enforce budgets, isolation or harness rules on those sessions; only brokered scopes still need approval.
      </Banner>

      {minted && (
        <Card title="Copy this token now" action={<span className="text-xs text-pill-human-fg">It won&apos;t be shown again</span>} bodyClassName="flex flex-col gap-3 px-[18px] py-4">
          <Command label="Token" text={minted.token} />
          <Command
            label="Claude Code"
            text={`claude mcp add --transport http lazyagents ${minted.mcp_url} --header "Authorization: Bearer ${minted.token}"`}
          />
          <Command
            label="Codex"
            text={`export LAZYAGENTS_TOKEN=${minted.token}\ncodex mcp add lazyagents --url ${minted.mcp_url} --bearer-token-env-var LAZYAGENTS_TOKEN`}
          />
          <p className="text-xs text-fg-tertiary">The Agent API at /mcp arrives with the rest of Phase 4; tokens you mint now will work with it.</p>
        </Card>
      )}

      <Card title="Tokens" bodyClassName="p-0">
        <form
          className="flex flex-wrap items-end gap-2 border-b border-border-subtle px-[18px] py-3"
          onSubmit={(e) => {
            e.preventDefault();
            create.mutate(label.trim(), { onSuccess: () => setLabel("") });
          }}
        >
          <Field label="Label" htmlFor="tok-label">
            <TextInput id="tok-label" className="w-64" maxLength={80} value={label} onChange={(e) => setLabel(e.target.value)} placeholder="e.g. Maya's laptop" />
          </Field>
          <Button type="submit" variant="primary" icon="key" disabled={create.isPending}>
            Create token
          </Button>
          {create.isError && <span role="alert" className="text-xs text-pill-blocked-fg">{create.error.message}</span>}
        </form>
        {!list.data ? (
          <div className="p-4"><Skeleton className="h-16" /></div>
        ) : list.data.tokens.length === 0 ? (
          <EmptyState icon="key" title="No tokens yet" className="py-8">
            Create one to connect a local Claude Code or Codex session as {detail.agent.name}.
          </EmptyState>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[560px] border-collapse text-[13px]">
              <thead>
                <tr className="text-left text-[12.5px] text-fg-tertiary">
                  <th className="px-[18px] py-2.5 font-normal">Label</th>
                  <th className="px-2 py-2.5 font-normal">Token</th>
                  <th className="px-2 py-2.5 font-normal">Created</th>
                  <th className="px-2 py-2.5 font-normal">Last used</th>
                  <th className="w-28" />
                </tr>
              </thead>
              <tbody>
                {list.data.tokens.map((t) => (
                  <tr key={t.id} className={cx("border-t border-border-row", t.revoked_at && "text-fg-tertiary")}>
                    <td className="px-[18px] py-2.5">{t.label || <span className="text-fg-tertiary">Untitled</span>}</td>
                    <td className="px-2 font-mono text-xs">{t.prefix}…</td>
                    <td className="px-2 text-xs text-fg-secondary">
                      {ago(t.created_at)}
                      {t.created_by && ` by ${t.created_by.name}`}
                    </td>
                    <td className="px-2 text-xs text-fg-secondary">{t.last_used_at ? ago(t.last_used_at) : "Never"}</td>
                    <td className="pr-[18px] text-right">
                      {t.revoked_at ? (
                        <span className="text-xs">Revoked</span>
                      ) : (
                        <Button variant="ghost" className="h-7 text-xs text-pill-blocked-fg" onClick={() => revoke.mutate(t.id)}>
                          Revoke
                        </Button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </div>
  );
}
