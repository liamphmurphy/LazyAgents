"use client";

import { DropdownMenu } from "radix-ui";
import { useRef, useState } from "react";
import { useBootstrap, useDeleteSecret, useSecrets, useSetSecret } from "@/lib/api/hooks";
import { cx } from "@/lib/cx";
import { ago } from "@/lib/format";
import { Button, IconButton } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { Banner, Card, Field, Skeleton, TextInput } from "@/components/ui/primitives";
import { MonoChip } from "@/components/domain/labels";
import { floatPanel } from "@/components/domain/pickers";

const CONNECTIONS = [
  { name: "harness:anthropic", label: "Anthropic", what: "Claude Code and OpenCode anthropic/… runs" },
  { name: "harness:openai", label: "OpenAI", what: "Codex and OpenCode openai/… runs" },
  { name: "git:ssh", label: "GitHub SSH key", what: "Clone and push over SSH" },
  { name: "git:pat", label: "GitHub", what: "Clone, push and open PRs" },
];

/**
 * Connections & secrets (settings.html). Values are write-only: the API
 * stores them encrypted and only returns names, grants and a 4-character
 * preview. A secret with a grant is injected only when that scope is allowed.
 */
export function SecretsCard() {
  const list = useSecrets();
  const boot = useBootstrap();
  const set = useSetSecret();
  const del = useDeleteSecret();
  const [form, setForm] = useState({ name: "", value: "", project: "" });
  const valueRef = useRef<HTMLInputElement>(null);
  const d = list.data;
  const disabled = d && !d.key_configured;
  const has = (name: string) => d?.secrets.some((s) => s.name === name && !s.project);

  function prefill(name: string) {
    setForm({ name, value: "", project: "" });
    valueRef.current?.focus();
  }

  return (
    <>
      {disabled && (
        <Banner tone="human" title="Secrets are read-only until SECRETS_KEY is set">
          Add <code>SECRETS_KEY=$(openssl rand -base64 32)</code> to <code>.env</code> and restart the API. The key encrypts every stored secret and never touches the database.
        </Banner>
      )}
      <Card title="Connections" bodyClassName="p-0">
        <ul>
          {CONNECTIONS.map((c, i) => (
            <li key={c.name} className={cx("flex items-center gap-3 px-[18px] py-3", i > 0 && "border-t border-border-row")}>
              <span className="grid size-8 place-items-center rounded-md border border-border-strong bg-selected text-fg-secondary">
                <Icon name={c.name === "git:pat" ? "github" : "key"} size={15} />
              </span>
              <span className="flex flex-1 flex-col">
                <span className="font-medium">{c.label}</span>
                <span className="text-xs text-fg-tertiary">
                  {c.what} · <span className="font-mono">{c.name}</span>
                </span>
              </span>
              {!d ? (
                <Skeleton className="h-4 w-20" />
              ) : has(c.name) ? (
                <span className="flex items-center gap-1.5 text-xs text-pill-done-fg">
                  <Icon name="check" size={12} /> Connected
                </span>
              ) : (
                <Button variant="outline" disabled={disabled} onClick={() => prefill(c.name)}>
                  Add key
                </Button>
              )}
            </li>
          ))}
        </ul>
      </Card>

      <Card title="Secrets" bodyClassName="p-0">
        <form
          className="grid grid-cols-[repeat(auto-fit,minmax(160px,1fr))] items-end gap-2 border-b border-border-subtle px-[18px] py-3"
          onSubmit={(e) => {
            e.preventDefault();
            set.mutate(
              { name: form.name.trim(), value: form.value, project_key: form.project || undefined },
              { onSuccess: () => setForm({ name: "", value: "", project: "" }) },
            );
          }}
        >
          <Field label="Name" htmlFor="sec-name">
            <TextInput id="sec-name" mono list="sec-known" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="db:staging" disabled={disabled} />
            <datalist id="sec-known">
              {d?.known.map((k) => <option key={k.name} value={k.name}>{k.description}</option>)}
            </datalist>
          </Field>
          <Field label="Value" htmlFor="sec-value">
            <TextInput
              id="sec-value"
              ref={valueRef}
              type="password"
              autoComplete="off"
              value={form.value}
              onChange={(e) => setForm({ ...form, value: e.target.value })}
              placeholder="Stored encrypted"
              disabled={disabled}
            />
          </Field>
          <Field label="Scope" htmlFor="sec-project">
            <DropdownMenu.Root>
              <DropdownMenu.Trigger asChild disabled={disabled}>
                <button id="sec-project" type="button" className="flex h-[30px] items-center gap-2 rounded-md border border-border-strong bg-surface px-2.5 text-left text-[13px] hover:bg-hover disabled:opacity-50">
                  <span className="flex-1 truncate">{boot.data?.projects.find((p) => p.key === form.project)?.name ?? "Workspace"}</span>
                  <Icon name="chev" size={12} className="text-fg-tertiary" />
                </button>
              </DropdownMenu.Trigger>
              <DropdownMenu.Portal>
                <DropdownMenu.Content align="start" sideOffset={4} className={cx(floatPanel, "w-56")}>
                  {[{ key: "", name: "Workspace", color: "" }, ...(boot.data?.projects ?? [])].map((p) => (
                    <DropdownMenu.Item key={p.key} onSelect={() => setForm({ ...form, project: p.key })} className="flex h-8 items-center gap-2.5 rounded-md px-2 outline-none data-[highlighted]:bg-hover">
                      {p.color && <span aria-hidden className="size-2.5 rounded-[3px]" style={{ background: p.color }} />}
                      {p.name}
                    </DropdownMenu.Item>
                  ))}
                </DropdownMenu.Content>
              </DropdownMenu.Portal>
            </DropdownMenu.Root>
          </Field>
          <Button type="submit" variant="primary" disabled={disabled || !form.name.trim() || !form.value || set.isPending}>
            Save secret
          </Button>
          {set.isError && <span role="alert" className="col-span-full text-xs text-pill-blocked-fg">{set.error.message}</span>}
        </form>
        {!d ? (
          <div className="p-4"><Skeleton className="h-16" /></div>
        ) : d.secrets.length === 0 ? (
          <p className="px-[18px] py-4 text-[13px] text-fg-tertiary">No secrets yet. Harness keys, the GitHub token and staging credentials go here.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[640px] border-collapse text-[13px]">
              <thead>
                <tr className="text-left text-[12.5px] text-fg-tertiary">
                  <th className="px-[18px] py-2.5 font-normal">Name</th>
                  <th className="px-2 py-2.5 font-normal">Scope</th>
                  <th className="px-2 py-2.5 font-normal">Backs</th>
                  <th className="px-2 py-2.5 font-normal">Value</th>
                  <th className="px-2 py-2.5 font-normal">Last used</th>
                  <th className="w-10" />
                </tr>
              </thead>
              <tbody>
                {d.secrets.map((s) => (
                  <tr key={s.id} className="border-t border-border-row">
                    <td className="px-[18px] py-2.5 font-mono text-[12.5px]">{s.name}</td>
                    <td className="whitespace-nowrap px-2">
                      {s.project ? (
                        <span className="flex items-center gap-1.5 text-fg-secondary">
                          <span aria-hidden className="size-2.5 rounded-[3px]" style={{ background: s.project.color }} />
                          {s.project.name}
                        </span>
                      ) : (
                        <span className="text-fg-secondary">Workspace</span>
                      )}
                    </td>
                    <td className="px-2">{s.grants_scope ? <MonoChip>{s.grants_scope}</MonoChip> : <span className="text-fg-tertiary">–</span>}</td>
                    <td className="px-2 font-mono text-xs text-fg-tertiary">•••• {s.preview}</td>
                    <td className="whitespace-nowrap px-2 text-xs text-fg-secondary">{s.last_used_at ? ago(s.last_used_at) : "Never"}</td>
                    <td className="pr-3">
                      <IconButton icon="trash" label={`Delete ${s.name}`} onClick={() => del.mutate(s.id)} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </>
  );
}
