"use client";

import Link from "next/link";
import { DropdownMenu } from "radix-ui";
import { useUpdateChat } from "@/lib/api/hooks";
import { useSpaces } from "@/lib/api/space-hooks";
import { cx } from "@/lib/cx";
import { listedSpaces, spaceHex } from "@/lib/spaces";
import { floatPanel } from "@/components/domain/pickers";
import { Icon } from "@/components/ui/icon";

const item = "flex h-8 cursor-default select-none items-center gap-2.5 rounded-md px-2 text-13 outline-none data-[highlighted]:bg-hover";

/**
 * The space a chat is in (LAZYAGENTS-213 frame D): its instructions, sources and memory reach the
 * chat, and the chat files tickets where the space does. Moving a chat starts a fresh session
 * with the conversation handed off, as a project change did. The chat reads every space's
 * tickets either way.
 */
type SpaceChat = { id: string; space?: { id: string; name: string; color: string }; revision: number; archived: boolean };

/** The spaces a chat can move to, and the move. */
export function useChatSpace(chat: SpaceChat, disabled: boolean) {
  const q = useSpaces();
  const update = useUpdateChat();
  const spaces = listedSpaces(q.data?.spaces ?? []);
  const locked = disabled || chat.archived || update.isPending;
  const pick = (space: string) => {
    if (space !== (chat.space?.id ?? "")) update.mutate({ id: chat.id, patch: { revision: chat.revision, space_id: space } });
  };
  return { spaces, current: chat.space, locked, pick, update };
}

/** No space, then every space, as radio items: inside the picker's menu or a submenu of a chat menu. */
export function SpaceRadioItems({ chat, spaces, onPick, itemClassName = item }: { chat: SpaceChat; spaces: ReturnType<typeof useChatSpace>["spaces"]; onPick: (space: string) => void; itemClassName?: string }) {
  return (
    <DropdownMenu.RadioGroup value={chat.space?.id ?? ""} onValueChange={onPick}>
      {spaces.map((s) => (
        <DropdownMenu.RadioItem key={s.id} value={s.id} className={itemClassName}>
          <span aria-hidden className="size-2.5 flex-none rounded-[3px]" style={{ background: spaceHex(s.color) }} />
          <span className="min-w-0 flex-1 truncate">{s.name}</span>
          {s.work && <span className="flex-none font-mono text-11 text-fg-tertiary">{s.work.key}</span>}
          <DropdownMenu.ItemIndicator><Icon name="check" size={14} className="text-accent" /></DropdownMenu.ItemIndicator>
        </DropdownMenu.RadioItem>
      ))}
      {spaces.length > 0 && <DropdownMenu.Separator className="my-1 h-px bg-border-strong" />}
      <DropdownMenu.RadioItem value="" className={itemClassName}>
        <span className="flex-1 text-fg-secondary">No space</span>
        <DropdownMenu.ItemIndicator><Icon name="check" size={14} className="text-accent" /></DropdownMenu.ItemIndicator>
      </DropdownMenu.RadioItem>
    </DropdownMenu.RadioGroup>
  );
}

/** "In ● Vesper ▾" in the chat's header: which space the chat is in, and moving it. */
export function ChatSpacePicker({ chat, disabled }: { chat: SpaceChat; disabled: boolean }) {
  const { spaces, current, locked, pick, update } = useChatSpace(chat, disabled);
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button
          type="button"
          data-testid="chat-space"
          disabled={locked}
          aria-label={current ? `In ${current.name}; move to another space` : "Put this chat in a space"}
          title={update.isError ? "Couldn’t move the chat. Try again." : current ? `${current.name}’s instructions, sources and memory reach this chat` : "Put the chat in a space to give it that space’s context"}
          className={cx(
            "flex min-w-0 flex-none items-center gap-1.5 rounded-full px-2 py-0.5 text-12 hover:bg-hover disabled:cursor-not-allowed disabled:opacity-60",
            current ? "text-fg-secondary" : "text-fg-tertiary",
            update.isError && "ring-1 ring-pill-blocked-fg",
          )}
        >
          <span className="text-fg-tertiary">In</span>
          {current ? <span aria-hidden className="size-2 flex-none rounded-[3px]" style={{ background: spaceHex(current.color) }} /> : null}
          <span className="max-w-[160px] truncate">{current?.name ?? "No space"}</span>
          <Icon name="chev" size={11} className="flex-none text-fg-tertiary" />
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align="start" sideOffset={6} className={cx(floatPanel, "w-[260px]")}>
          <SpaceRadioItems chat={chat} spaces={spaces} onPick={pick} />
          {current && (
            <>
              <DropdownMenu.Separator className="my-1 h-px bg-border-strong" />
              <DropdownMenu.Item asChild className={item}>
                <Link href={`/spaces/${current.id}`}>
                  <span className="flex-1">Open {current.name}</span>
                  <Icon name="chevr" size={12} className="text-fg-tertiary" />
                </Link>
              </DropdownMenu.Item>
            </>
          )}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}
