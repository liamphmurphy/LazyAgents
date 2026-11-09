"use client";

import { DropdownMenu } from "radix-ui";
import { useBootstrap, useUpdateChat } from "@/lib/api/hooks";
import { cx } from "@/lib/cx";
import { floatPanel } from "@/components/domain/pickers";
import { Icon } from "@/components/ui/icon";

const item = "flex h-8 cursor-default select-none items-center gap-2.5 rounded-md px-2 text-[13px] outline-none data-[highlighted]:bg-hover";

/** Gives this chat access to one project's tickets, epics and pages, or to none. */
export function ChatProjectPicker({ chat, disabled }: { chat: { id: string; project?: string; revision: number; archived: boolean }; disabled: boolean }) {
  const boot = useBootstrap();
  const update = useUpdateChat();
  const projects = (boot.data?.projects ?? []).filter((p) => !p.archived || p.key === chat.project);
  const current = projects.find((p) => p.key === chat.project);
  const locked = disabled || chat.archived || update.isPending;
  const pick = (project: string) => {
    if (project !== (chat.project ?? "")) update.mutate({ id: chat.id, patch: { revision: chat.revision, project } });
  };
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button
          type="button"
          data-testid="chat-project"
          disabled={locked}
          aria-label={current ? `Project access: ${current.name}` : "Attach a project"}
          title={update.isError ? "Couldn’t change the project. Try again." : current ? "This chat can use this project" : "Give this chat access to a project"}
          className={cx(
            "flex min-w-0 flex-none items-center gap-1.5 rounded-full border border-border bg-surface px-2.5 py-0.5 text-xs text-fg-secondary hover:bg-hover disabled:cursor-not-allowed disabled:opacity-60",
            update.isError && "border-pill-blocked-fg",
          )}
        >
          {current ? <span aria-hidden className="size-2 flex-none rounded-[3px]" style={{ background: current.color }} /> : <Icon name="plus" size={12} className="flex-none" />}
          <span className="truncate">{current?.name ?? "Add project"}</span>
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align="start" sideOffset={6} className={cx(floatPanel, "w-[240px]")}>
          <DropdownMenu.RadioGroup value={chat.project ?? ""} onValueChange={pick}>
            <DropdownMenu.RadioItem value="" className={item}>
              <span className="flex-1 text-fg-secondary">No project</span>
              <DropdownMenu.ItemIndicator><Icon name="check" size={14} className="text-accent" /></DropdownMenu.ItemIndicator>
            </DropdownMenu.RadioItem>
            <DropdownMenu.Separator className="my-1 h-px bg-border-strong" />
            {projects.map((p) => (
              <DropdownMenu.RadioItem key={p.key} value={p.key} className={item}>
                <span aria-hidden className="size-2.5 flex-none rounded-[3px]" style={{ background: p.color }} />
                <span className="flex-1 truncate">{p.name}</span>
                <DropdownMenu.ItemIndicator><Icon name="check" size={14} className="text-accent" /></DropdownMenu.ItemIndicator>
              </DropdownMenu.RadioItem>
            ))}
          </DropdownMenu.RadioGroup>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}
