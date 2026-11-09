"use client";

import { useRouter } from "next/navigation";
import { useCreateChat } from "@/lib/api/hooks";
import { attachQuery, type ContextRef } from "@/lib/chat-context";
import { Button } from "@/components/ui/button";
import { NoHarnessNote, useHarnessHealth } from "@/components/domain/setup";

/**
 * Opens a new chat with the item attached as a chip and its project, if it has
 * one, attached to the chat.
 */
export function StartChatButton({ attach, title, project }: { attach: ContextRef; title: string; project?: string }) {
  const router = useRouter();
  const create = useCreateChat();
  const health = useHarnessHealth();
  const noHarness = !!health && !health.ready;
  return (
    <>
      <Button
        variant="outline"
        icon="msg"
        disabled={noHarness || create.isPending}
        onClick={() =>
          create.mutate(
            project ? { project } : {},
            { onSuccess: (c) => router.push(`/chat/${c.id}?${attachQuery(project, attach, title)}`) },
          )
        }
      >
        Start chat
      </Button>
      {noHarness && <NoHarnessNote health={health} />}
      {create.isError && (
        <span role="alert" className="text-xs text-pill-blocked-fg">
          {create.error instanceof Error ? create.error.message : "Couldn’t start the chat."}
        </span>
      )}
    </>
  );
}
