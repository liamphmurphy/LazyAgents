"use client";

import { DropdownMenu } from "radix-ui";
import { useRouter } from "next/navigation";
import { useRef, useState, type FormEvent } from "react";
import type { Schemas } from "@/lib/api/client";
import { useUpdateChat } from "@/lib/api/hooks";
import { chatTitle } from "@/lib/chat-ui";
import { cx } from "@/lib/cx";
import { repoShortName } from "@/lib/persona-ui";
import { floatPanel } from "@/components/domain/pickers";
import { Icon } from "@/components/ui/icon";
import { TextInput } from "@/components/ui/primitives";
import { ProjectRadioItems, useChatProject } from "./chat-project";
import { useChatPanels } from "./chat-panel-host";
import { HistoryDrawer } from "./history-drawer";
import type { PersonaLook } from "./persona";

type Chat = Schemas["ChatSession"];
const errorText = (e: unknown) => (e instanceof Error ? e.message : "Something went wrong. Please try again.");

/** Every control in the header is a 44px touch target. */
const target = "relative grid size-[44px] flex-none place-items-center rounded-md text-fg-secondary hover:bg-hover hover:text-fg disabled:opacity-50";
const item = "flex min-h-[44px] cursor-default select-none items-center gap-2.5 rounded-md px-2.5 text-[15px] outline-none data-[highlighted]:bg-hover data-[disabled]:opacity-50";

/**
 * The chat header below COMPACT (DESIGN.md §3 Responsive): ≡ opens the history drawer, then the
 * title with who it's with and the repository underneath, the panels button (a dot while a design,
 * canvas or playground is open) and ··· with what the desktop header and tabs hold.
 */
export function CompactChatHeader({ chat: c, personas, tab, onTab, running, canCode, onRepository, onRestored }: {
  chat: Chat;
  personas: PersonaLook[];
  tab: "chat" | "activity";
  onTab: (t: "chat" | "activity") => void;
  running: boolean;
  canCode: boolean;
  onRepository: () => void;
  onRestored: () => void;
}) {
  const router = useRouter();
  const update = useUpdateChat();
  const panels = useChatPanels(c.id, running);
  const project = useChatProject(c, running);
  const [editing, setEditing] = useState(false);
  const [title, setTitle] = useState(c.title);
  // Rename and Repository move focus on (the title box, the repository picker) once the menu has
  // closed, rather than it handing focus back to ··· and closing the picker it opened.
  const afterClose = useRef<(() => void) | null>(null);
  const label = chatTitle(c);
  const who = personas.map((p) => p.name).join(", ");
  const panelOpen = panels.open.length > 0;
  const showPanel = panels.active ?? panels.available[0];

  function save(e: FormEvent) {
    e.preventDefault();
    const t = title.trim();
    if (!t || t === c.title) return setEditing(false);
    update.mutateAsync({ id: c.id, patch: { revision: c.revision, title: t } }).then(() => setEditing(false), () => undefined);
  }
  const archive = (archived: boolean) =>
    update.mutateAsync({ id: c.id, patch: { revision: c.revision, archived } }).then(() => (archived ? router.push("/chat") : onRestored()), () => undefined);

  return (
    <div data-testid="compact-chat-header" className="flex min-h-[52px] flex-col border-b border-border-subtle px-1 py-1">
      <div className="flex items-center gap-1">
        <HistoryDrawer
          trigger={
            <button type="button" aria-label="Chat history" className={target}>
              <Icon name="sidebar" size={20} strokeWidth={1.6} />
            </button>
          }
        />
        {editing ? (
          <form onSubmit={save} className="flex min-w-0 flex-1 items-center gap-1">
            <TextInput aria-label="Conversation title" className="!h-[44px] min-w-0 flex-1 !text-[16px]" value={title} placeholder="New conversation" autoFocus onChange={(e) => setTitle(e.target.value)} onKeyDown={(e) => e.key === "Escape" && setEditing(false)} />
            <button type="submit" aria-label="Save title" className={target} disabled={update.isPending}>
              <Icon name="check" size={18} />
            </button>
            <button type="button" aria-label="Cancel rename" className={target} onClick={() => setEditing(false)}>
              <Icon name="x" size={18} />
            </button>
          </form>
        ) : (
          <div className="min-w-0 flex-1 px-1">
            <h1 className="truncate text-[15px] font-semibold leading-tight">{label}</h1>
            <p data-testid="chat-subtitle" className="truncate text-xs text-fg-tertiary">
              {who}
              {c.repo_url && <> · <span className="font-mono">{repoShortName(c.repo_url)}</span></>}
              {c.archived && " · Archived"}
            </p>
          </div>
        )}
        {!editing && (
          <>
            <button
              type="button"
              aria-label={panelOpen ? "Panels (open)" : "Panels"}
              disabled={!showPanel}
              onClick={() => showPanel && panels.switchTo(showPanel)}
              className={target}
            >
              <Icon name="expand" size={18} strokeWidth={1.6} />
              {panelOpen && <span aria-hidden data-testid="panels-dot" className="absolute right-2 top-2 size-2 rounded-full bg-accent" />}
            </button>
            <DropdownMenu.Root>
              <DropdownMenu.Trigger asChild>
                <button type="button" aria-label="Chat menu" className={target}>
                  <Icon name="dots" size={18} />
                </button>
              </DropdownMenu.Trigger>
              <DropdownMenu.Portal>
                <DropdownMenu.Content
                  align="end"
                  sideOffset={4}
                  className={cx(floatPanel, "w-[min(280px,calc(100vw-16px))]")}
                  onCloseAutoFocus={(e) => {
                    const run = afterClose.current;
                    afterClose.current = null;
                    if (!run) return;
                    e.preventDefault();
                    run();
                  }}
                >
                  <DropdownMenu.CheckboxItem className={item} checked={tab === "activity"} onCheckedChange={(on) => onTab(on ? "activity" : "chat")}>
                    <Icon name="history" size={16} />
                    <span className="flex-1">Activity</span>
                    <DropdownMenu.ItemIndicator><Icon name="check" size={16} className="text-accent" /></DropdownMenu.ItemIndicator>
                  </DropdownMenu.CheckboxItem>
                  {canCode && !c.archived && (
                    <DropdownMenu.Item className={item} disabled={running} onSelect={() => (afterClose.current = onRepository)}>
                      <Icon name="github" size={16} />
                      <span className="flex-1">Repository</span>
                      <span className="max-w-[40%] truncate text-xs text-fg-tertiary">{c.repo_url ? repoShortName(c.repo_url) : "None"}</span>
                    </DropdownMenu.Item>
                  )}
                  {!c.space && (
                    <>
                      <DropdownMenu.Separator className="my-1 h-px bg-border-strong" />
                      <DropdownMenu.Label className="px-2.5 pb-1 pt-1.5 text-xs font-medium text-fg-tertiary">Default project</DropdownMenu.Label>
                      {project.locked ? (
                        <p className="px-2.5 pb-1.5 text-[13px] text-fg-secondary">{project.current?.name ?? "No default"}</p>
                      ) : (
                        <ProjectRadioItems chat={c} projects={project.projects} onPick={project.pick} itemClassName={item} />
                      )}
                    </>
                  )}
                  <DropdownMenu.Separator className="my-1 h-px bg-border-strong" />
                  <DropdownMenu.Item
                    className={item}
                    onSelect={() => {
                      afterClose.current = () => {
                        setTitle(c.title);
                        setEditing(true);
                      };
                    }}
                  >
                    <Icon name="edit" size={16} />
                    Rename
                  </DropdownMenu.Item>
                  <DropdownMenu.Item className={item} disabled={update.isPending} onSelect={() => archive(!c.archived)}>
                    <Icon name={c.archived ? "refresh" : "folder"} size={16} />
                    {c.archived ? "Restore" : "Archive"}
                  </DropdownMenu.Item>
                </DropdownMenu.Content>
              </DropdownMenu.Portal>
            </DropdownMenu.Root>
          </>
        )}
      </div>
      {update.isError && (
        <p role="alert" className="px-2 pb-1 text-xs text-pill-blocked-fg">
          {errorText(update.error)}
        </p>
      )}
    </div>
  );
}
