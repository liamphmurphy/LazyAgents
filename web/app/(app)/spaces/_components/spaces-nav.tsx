"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { Popover } from "radix-ui";
import { useState } from "react";
import { useChat, usePersonaDock } from "@/lib/api/hooks";
import { useSpaceChats, useSpaces } from "@/lib/api/space-hooks";
import { chatTitle } from "@/lib/chat-ui";
import { cx } from "@/lib/cx";
import { activityLabel, badgeTarget, groupSpaceActivity, jumpTarget, type SpaceActivity, type SpaceChat } from "@/lib/space-activity";
import { NESTED_CHATS } from "@/lib/spaces";
import { Icon } from "@/components/ui/icon";
import { NewSpaceDialog } from "./new-space-dialog";
import { SpaceTile } from "./space-parts";

/** The space the menu shows as open: the one whose page is open, or the open chat's. */
function useOpenSpace(viewed: string | null) {
  const pathname = usePathname();
  const chat = useChat(viewed ?? undefined);
  const onPage = pathname.match(/^\/spaces\/([^/]+)/)?.[1];
  return { page: onPage ? decodeURIComponent(onPage) : undefined, chatSpace: chat.data?.space?.id, chatId: viewed ?? undefined };
}

/** The newest chats of the open chat's space, nested under it (mockup 2). */
function NestedChats({ spaceId, chatId, total }: { spaceId: string; chatId?: string; total: number }) {
  const chats = useSpaceChats(spaceId);
  const list = (chats.data?.chats ?? []).slice(0, NESTED_CHATS);
  // The open chat always shows, even when it isn't among the newest.
  const open = chatId && !list.some((c) => c.id === chatId) ? chats.data?.chats.find((c) => c.id === chatId) : undefined;
  const rows = open ? [...list.slice(0, NESTED_CHATS - 1), open] : list;
  return (
    <ul className="relative ml-[17px] flex flex-col gap-px border-l border-border-strong pl-[6px]" aria-label="Chats in this space">
      {rows.map((c) => (
        <li key={c.id}>
          <Link
            href={`/chat/${c.id}`}
            aria-current={c.id === chatId ? "page" : undefined}
            className={cx(
              "flex h-[26px] items-center truncate rounded-md px-[8px] text-13",
              c.id === chatId ? "bg-accent-soft text-fg" : "text-fg-secondary hover:bg-hover hover:text-fg",
            )}
          >
            <span className="truncate">{chatTitle(c)}</span>
          </Link>
        </li>
      ))}
      <li>
        <Link href={`/spaces/${spaceId}`} className="flex h-[26px] items-center rounded-md px-[8px] text-12 text-fg-tertiary hover:bg-hover hover:text-fg">
          See all {total}
        </Link>
      </li>
    </ul>
  );
}

const dotClass = (state: SpaceChat["state"]) => (state === "needs_you" ? "bg-state-human" : "bg-state-done");

/** The activity badge: one chat opens it, several open a flyout listing them, latest started first. */
function ActivityBadge({ spaceId, name, activity }: { spaceId: string; name: string; activity: SpaceActivity }) {
  const [open, setOpen] = useState(false);
  const target = badgeTarget(activity);
  const label = activityLabel(name, activity);
  const dot = <span aria-hidden data-space-activity className={cx("size-1.5 flex-none rounded-full", activity.needsYou ? "bg-state-human" : "bg-state-done")} />;
  const pill = "flex h-[20px] flex-none items-center gap-[5px] rounded-full bg-hover px-[7px] text-12 text-fg-secondary hover:text-fg";
  if (target.kind === "chat") {
    return (
      <Link href={`/chat/${target.chat.id}`} aria-label={`${label}; open ${chatTitle(target.chat)}`} title={target.chat.title} className={pill}>
        {dot}1
      </Link>
    );
  }
  return (
    <Popover.Root open={open} onOpenChange={setOpen}>
      <Popover.Trigger asChild>
        <button type="button" aria-label={`${label}; show chats`} className={pill}>
          {dot}
          {activity.chats.length}
        </button>
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content side="right" align="start" sideOffset={8} collisionPadding={8} aria-label={`Chats active in ${name}`}
          className="z-50 w-[300px] rounded-xl border border-border-strong bg-surface p-[6px] shadow-lg">
          <p className="px-[8px] pb-[4px] pt-[2px] text-11 font-medium uppercase tracking-wide text-fg-tertiary">Active in {name}</p>
          <ul className="flex flex-col gap-px">
            {activity.chats.map((c) => (
              <li key={c.id}>
                <Link href={`/chat/${c.id}`} onClick={() => setOpen(false)} className="flex flex-col gap-px rounded-md px-[8px] py-[6px] hover:bg-hover">
                  <span className="flex items-center gap-[8px] text-13 text-fg">
                    <span aria-hidden className={cx("size-1.5 flex-none rounded-full", dotClass(c.state))} />
                    <span className="min-w-[0px] flex-1 truncate">{chatTitle(c)}</span>
                    {c.id === activity.latestWorking?.id && <span className="flex-none text-11 text-fg-tertiary">latest</span>}
                  </span>
                  <span className="truncate pl-[14px] text-12 text-fg-tertiary">
                    {c.persona ? `${c.persona.name} · ` : ""}
                    {c.state === "needs_you" ? "waiting on you" : "working"}
                  </span>
                </Link>
              </li>
            ))}
          </ul>
          <Link href={`/spaces/${spaceId}`} onClick={() => setOpen(false)} className="mt-[4px] block rounded-md border-t border-border-subtle px-[8px] pb-[4px] pt-[8px] text-12 text-accent hover:text-fg">
            Open {name}
          </Link>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}

/** Chat mode's Spaces section (mockups 1 and 2): every space, with the open chat's space showing its newest chats. */
/** The menu's spaces. hideWhenEmpty: nothing until there is one (the menu offers "New space" elsewhere). */
export function SidebarSpaces({ viewed, hideWhenEmpty }: { viewed: string | null; hideWhenEmpty?: boolean }) {
  const spaces = useSpaces();
  const open = useOpenSpace(viewed);
  const dock = usePersonaDock();
  const activity = groupSpaceActivity(dock.data?.space_chats);
  const [creating, setCreating] = useState(false);
  const list = spaces.data?.spaces ?? [];
  if (hideWhenEmpty && !spaces.isError && list.length === 0) return null;
  return (
    <section aria-label="Spaces" className="flex flex-col gap-px">
      <div className="flex items-center pb-[4px] pl-[8px] pr-[4px]">
        <span className="flex-1 text-12 font-medium text-fg-tertiary">Spaces</span>
        <button
          type="button"
          aria-label="New space"
          title="New space"
          onClick={() => setCreating(true)}
          className="grid size-[20px] place-items-center rounded text-fg-secondary hover:bg-hover hover:text-fg"
        >
          <Icon name="plus" size={13} strokeWidth={1.8} />
        </button>
      </div>
      {spaces.isError ? (
        <p role="alert" className="px-[8px] text-12 text-pill-blocked-fg">Couldn’t load spaces.</p>
      ) : spaces.data && list.length === 0 ? (
        <button type="button" onClick={() => setCreating(true)} className="flex h-[30px] items-center gap-[10px] rounded-md px-[8px] text-left text-13 text-fg-tertiary hover:bg-hover hover:text-fg">
          <span className="grid size-[16px] place-items-center rounded border border-dashed border-border-strong">
            <Icon name="plus" size={10} />
          </span>
          New space
        </button>
      ) : (
        list.map((s) => {
          const expanded = !!open.chatSpace && open.chatSpace === s.id && !open.page;
          const active = open.page === s.id;
          const busy = activity.get(s.id);
          const jump = busy && jumpTarget(busy);
          return (
            <div key={s.id} className="flex flex-col gap-px">
              <div
                className={cx(
                  "group flex h-[28px] items-center gap-[6px] rounded-md pr-[8px] text-13",
                  active ? "bg-accent-soft text-fg" : expanded ? "text-fg hover:bg-hover" : "text-fg-secondary hover:bg-hover hover:text-fg",
                )}
              >
                <Link
                  href={`/spaces/${s.id}`}
                  aria-current={active ? "page" : undefined}
                  aria-expanded={expanded || undefined}
                  className="flex h-full min-w-[0px] flex-1 items-center gap-[10px] pl-[8px]"
                >
                  <SpaceTile name={s.name} color={s.color} />
                  <span className="min-w-[0px] flex-1 truncate">{s.name}</span>
                </Link>
                {jump && (
                  <Link
                    href={`/chat/${jump.id}`}
                    aria-label={`Jump to ${chatTitle(jump)}, the latest working chat in ${s.name}`}
                    title="Jump to the latest working chat"
                    className="grid size-[20px] flex-none place-items-center rounded-full text-fg-tertiary opacity-0 hover:bg-hover hover:text-fg focus-visible:opacity-100 group-hover:opacity-100"
                  >
                    <Icon name="chevr" size={12} strokeWidth={1.8} />
                  </Link>
                )}
                {busy && <ActivityBadge spaceId={s.id} name={s.name} activity={busy} />}
                {expanded ? (
                  <Icon name="chev" size={11} className="flex-none text-fg-tertiary" />
                ) : (
                  !busy && <span className="flex-none text-12 text-fg-tertiary">{s.chat_count}</span>
                )}
              </div>
              {expanded && <NestedChats spaceId={s.id} chatId={open.chatId} total={s.chat_count} />}
            </div>
          );
        })
      )}
      <NewSpaceDialog open={creating} onOpenChange={setCreating} />
    </section>
  );
}

