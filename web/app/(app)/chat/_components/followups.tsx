"use client";

import { useChatFollowups, useDeleteFollowup } from "@/lib/api/hooks";
import { Icon } from "@/components/ui/icon";

/**
 * Follow-ups queued behind the running reply, above the message box. Each goes
 * out on its own once a reply ends. After a stop or a failure they're held:
 * Send now (which interrupts while a reply runs), Edit (back into the box) or
 * remove.
 */
export function FollowupTray({
  chatId,
  name,
  running,
  held,
  onSendNow,
  onEdit,
  disabled,
}: {
  chatId: string;
  name: string;
  running: boolean;
  /** The last reply was stopped or failed, so nothing goes out on its own. */
  held: boolean;
  onSendNow: (body: string, done: () => void) => void;
  onEdit: (body: string) => void;
  disabled?: boolean;
}) {
  const list = useChatFollowups(chatId);
  const remove = useDeleteFollowup(chatId);
  const items = list.data ?? [];
  if (items.length === 0) return null;
  const header = running
    ? `Queued · sends when ${name} finishes, in order`
    : held
      ? `Held · ${name} stopped, so these wait for you`
      : `Queued · sends after ${name}'s next reply`;
  return (
    <section aria-label="Queued follow-ups" data-testid="followup-tray" className="mb-2 overflow-hidden rounded-xl border border-border bg-surface">
      <div className="flex items-center gap-2 border-b border-border-subtle px-3 py-2 text-xs text-fg-secondary">
        <Icon name="clock" size={12} />
        <span className="min-w-0 truncate">{header}</span>
        <span className="ml-auto font-mono text-fg-tertiary">{items.length}</span>
      </div>
      <ol>
        {items.map((f, i) => (
          <li key={f.id} data-testid="followup" className="flex items-center gap-2.5 border-b border-border-row py-1.5 pl-3 pr-2 last:border-b-0">
            <span aria-hidden className="w-3.5 font-mono text-[11px] text-fg-tertiary">{i + 1}</span>
            <span className="min-w-0 flex-1 truncate text-[13px] text-fg-2" title={f.body}>
              {f.body}
            </span>
            <button
              type="button"
              disabled={disabled || remove.isPending}
              title={running ? `Stop ${name} and send this now` : "Send this now"}
              onClick={() => onSendNow(f.body, () => remove.mutate(f.id))}
              className="h-6 flex-none rounded-md px-2 text-xs text-fg-tertiary hover:bg-hover hover:text-fg disabled:opacity-50"
            >
              Send now
            </button>
            <button
              type="button"
              disabled={remove.isPending}
              onClick={() => remove.mutate(f.id, { onSuccess: () => onEdit(f.body) })}
              className="h-6 flex-none rounded-md px-2 text-xs text-fg-tertiary hover:bg-hover hover:text-fg disabled:opacity-50"
            >
              Edit
            </button>
            <button
              type="button"
              aria-label={`Remove follow-up ${i + 1}`}
              disabled={remove.isPending}
              onClick={() => remove.mutate(f.id)}
              className="grid size-6 flex-none place-items-center rounded-md text-fg-tertiary hover:bg-hover hover:text-fg disabled:opacity-50"
            >
              <Icon name="x" size={12} />
            </button>
          </li>
        ))}
      </ol>
      {remove.isError && (
        <p role="alert" className="px-3 pb-2 text-xs text-pill-blocked-fg">
          {remove.error instanceof Error ? remove.error.message : "Couldn’t remove it. Try again."}
        </p>
      )}
    </section>
  );
}
