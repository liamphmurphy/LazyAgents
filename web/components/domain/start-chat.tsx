"use client";

import { useRouter } from "next/navigation";
import { useBootstrap, useCreateChat } from "@/lib/api/hooks";
import { attachQuery, type ContextRef } from "@/lib/chat-context";
import { Button } from "@/components/ui/button";

/**
 * Opens a new chat in the item's project with the item attached as a chip.
 * Workspace-level pages (no project) use the first active project.
 */
export function StartChatButton({ attach, title, project }: { attach: ContextRef; title: string; project?: string }) {
  const router = useRouter();
  const boot = useBootstrap();
  const create = useCreateChat();
  const key = project ?? boot.data?.projects.find((p) => !p.archived)?.key;
  return (
    <>
      <Button
        variant="outline"
        icon="msg"
        disabled={!key || create.isPending}
        onClick={() =>
          key &&
          create.mutate(
            { project: key },
            { onSuccess: (c) => router.push(`/chat/${c.id}?${attachQuery(key, attach, title)}`) },
          )
        }
      >
        Start chat
      </Button>
      {create.isError && (
        <span role="alert" className="text-xs text-pill-blocked-fg">
          {create.error instanceof Error ? create.error.message : "Couldn’t start the chat."}
        </span>
      )}
    </>
  );
}
