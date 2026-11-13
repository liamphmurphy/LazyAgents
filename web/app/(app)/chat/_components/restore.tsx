"use client";

import type { Schemas } from "@/lib/api/client";
import { useUpdateChat } from "@/lib/api/hooks";
import { chatTitle } from "@/lib/chat-ui";
import { Button, type ButtonVariant } from "@/components/ui/button";

const errorText = (e: unknown) => (e instanceof Error ? e.message : "Something went wrong. Please try again.");

/** Un-archives a chat (PATCH archived:false, revision-checked); it moves back to the active list. */
export function RestoreChat({
  chat,
  variant = "outline",
  compact,
  onRestored,
}: {
  chat: Pick<Schemas["ChatSession"], "id" | "title" | "revision">;
  variant?: ButtonVariant;
  compact?: boolean;
  onRestored?: () => void;
}) {
  const update = useUpdateChat();
  return (
    <span className="flex flex-col items-end gap-0.5">
      <Button
        variant={variant}
        icon={compact ? undefined : "refresh"}
        aria-label={`Restore ${chatTitle(chat)}`}
        // cx doesn't merge Tailwind classes, so the compact size overrides with !important.
        className={compact ? "!h-6 !px-2 !text-12" : undefined}
        disabled={update.isPending}
        onClick={() => update.mutateAsync({ id: chat.id, patch: { revision: chat.revision, archived: false } }).then(onRestored, () => undefined)}
      >
        Restore
      </Button>
      {update.isError && (
        <span role="alert" className="text-12 text-pill-blocked-fg">
          {errorText(update.error)}
        </span>
      )}
    </span>
  );
}
