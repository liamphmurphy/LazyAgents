"use client";

import Link from "next/link";
import { Popover } from "radix-ui";
import { useApiSecretApprovals, useChatApiSecrets, useDisableChatApiSecret } from "@/lib/api/secret-hooks";
import { cx } from "@/lib/cx";
import { ago } from "@/lib/format";
import { Icon } from "@/components/ui/icon";

/**
 * The chat header's sign that a secret was used here: hidden until one is,
 * amber while a request waits for the person. Opening it lists each secret's
 * use in this chat and turns one off for the chat.
 */
export function SecretsChip({ chatId }: { chatId: string }) {
  const used = useChatApiSecrets(chatId);
  const waiting = useApiSecretApprovals(chatId);
  const disable = useDisableChatApiSecret(chatId);
  const rows = used.data?.secrets ?? [];
  const pending = waiting.data?.approvals ?? [];
  if (!rows.length && !pending.length) return null;
  const live = pending.length > 0;
  const label = live ? `${pending[0].secret_handle} waiting` : rows.length === 1 ? `${rows[0].handle} used ${rows[0].calls}×` : `${rows.length} secrets used`;
  return (
    <Popover.Root>
      <Popover.Trigger asChild>
        <button
          type="button"
          data-testid="secrets-chip"
          className={cx(
            "flex min-w-0 flex-none items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-xs max-md:hidden @max-[640px]:hidden",
            live ? "border-pill-human-fg/50 bg-pill-human-bg text-pill-human-fg" : "border-border bg-surface text-pill-human-fg hover:bg-hover",
          )}
        >
          <Icon name="key" size={13} className="flex-none" />
          <span className="truncate">{label}</span>
          {live && <span className="size-1.5 flex-none animate-pulse rounded-full bg-current" aria-hidden />}
        </button>
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content align="end" sideOffset={6} className="z-50 w-[280px] rounded-xl border border-border-strong bg-surface-raised p-3 shadow-float" aria-label="Secrets in this chat">
          <p className="mb-2 text-[13px] font-medium text-fg">Secrets in this chat</p>
          {live && <p className="mb-2 text-xs text-pill-human-fg">A request is waiting for you below.</p>}
          <ul className="flex flex-col gap-2">
            {rows.map((s) => (
              <li key={s.id} className="flex flex-col gap-0.5 border-t border-border-subtle pt-2 first:border-t-0 first:pt-0">
                <span className="flex items-center gap-1.5 text-[13px] text-fg"><Icon name="key" size={13} className="text-pill-human-fg" />{s.name}</span>
                <span className="text-xs text-fg-secondary">{s.calls} {s.calls === 1 ? "request" : "requests"} · {ago(s.last_used_at)} · {s.methods.join(", ")}{!s.enabled && " · off here"}</span>
                <span className="mt-1 flex gap-3 text-[12px]">
                  <Link href={`/customization?who=secrets&secret=${s.id}`} className="text-accent hover:underline">View activity</Link>
                  {s.enabled && (
                    <button type="button" className="text-pill-blocked-fg hover:underline disabled:opacity-50" disabled={disable.isPending} onClick={() => disable.mutate(s.id)}>
                      Turn off for this chat
                    </button>
                  )}
                </span>
              </li>
            ))}
          </ul>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}
