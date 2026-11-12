"use client";

import { DropdownMenu } from "radix-ui";
import { useBootstrap, useUpdateChat } from "@/lib/api/hooks";
import { cx } from "@/lib/cx";
import { floatPanel } from "@/components/domain/pickers";
import { Icon } from "@/components/ui/icon";

const item = "flex h-8 cursor-default select-none items-center gap-2.5 rounded-md px-2 text-[13px] outline-none data-[highlighted]:bg-hover";

/**
 * The chat's default project: where new tickets and epics go unless the persona names another.
 * The chat reads every project either way. dashed: the chip among a space chat's context chips.
 */
type ProjectChat = { id: string; project?: string; revision: number; archived: boolean };

/** The projects a chat can default to (archived ones only if it already does), and the move to another. */
export function useChatProject(chat: ProjectChat, disabled: boolean) {
  const boot = useBootstrap();
  const update = useUpdateChat();
  const projects = (boot.data?.projects ?? []).filter((p) => !p.archived || p.key === chat.project);
  const current = projects.find((p) => p.key === chat.project);
  const locked = disabled || chat.archived || update.isPending;
  const pick = (project: string) => {
    if (project !== (chat.project ?? "")) update.mutate({ id: chat.id, patch: { revision: chat.revision, project } });
  };
  return { projects, current, locked, pick, update };
}

/** No default, then every project, as radio items: inside the picker's menu or a submenu of the compact chat menu. */
export function ProjectRadioItems({ chat, projects, onPick, itemClassName = item }: { chat: ProjectChat; projects: ReturnType<typeof useChatProject>["projects"]; onPick: (project: string) => void; itemClassName?: string }) {
  return (
    <DropdownMenu.RadioGroup value={chat.project ?? ""} onValueChange={onPick}>
      <DropdownMenu.RadioItem value="" className={itemClassName}>
        <span className="flex-1 text-fg-secondary">No default</span>
        <DropdownMenu.ItemIndicator><Icon name="check" size={14} className="text-accent" /></DropdownMenu.ItemIndicator>
      </DropdownMenu.RadioItem>
      <DropdownMenu.Separator className="my-1 h-px bg-border-strong" />
      {projects.map((p) => (
        <DropdownMenu.RadioItem key={p.key} value={p.key} className={itemClassName}>
          <span aria-hidden className="size-2.5 flex-none rounded-[3px]" style={{ background: p.color }} />
          <span className="flex-1 truncate">{p.name}</span>
          <DropdownMenu.ItemIndicator><Icon name="check" size={14} className="text-accent" /></DropdownMenu.ItemIndicator>
        </DropdownMenu.RadioItem>
      ))}
    </DropdownMenu.RadioGroup>
  );
}

export function ChatProjectPicker({ chat, disabled, dashed }: { chat: ProjectChat; disabled: boolean; dashed?: boolean }) {
  const { projects, current, locked, pick, update } = useChatProject(chat, disabled);
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button
          type="button"
          data-testid="chat-project"
          disabled={locked}
          aria-label={current ? `Default project: ${current.name}` : "Set a default project"}
          title={update.isError ? "Couldn’t change the project. Try again." : current ? "New tickets go here unless you say otherwise" : "Pick where new tickets go. The chat reads every project either way."}
          className={cx(
            "flex min-w-0 flex-none items-center gap-1.5 rounded-full border text-xs text-fg-secondary hover:bg-hover disabled:cursor-not-allowed disabled:opacity-60",
            dashed ? "h-[26px] px-3" : "border-border bg-surface px-2.5 py-0.5",
            dashed && (current ? "border-border-strong bg-surface-raised" : "border-dashed border-border-strong text-fg-tertiary"),
            update.isError && "border-pill-blocked-fg",
          )}
        >
          {current ? <span aria-hidden className="size-2 flex-none rounded-[3px]" style={{ background: current.color }} /> : <Icon name="plus" size={12} className="flex-none" />}
          <span className="truncate">{current?.name ?? "Default project"}</span>
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align="start" sideOffset={6} className={cx(floatPanel, "w-[240px]")}>
          <ProjectRadioItems chat={chat} projects={projects} onPick={pick} />
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}
