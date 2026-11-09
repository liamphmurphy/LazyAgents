"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useState } from "react";
import { useChat } from "@/lib/api/hooks";
import { useSpaceChats, useSpaces } from "@/lib/api/space-hooks";
import { chatTitle } from "@/lib/chat-ui";
import { cx } from "@/lib/cx";
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
    <ul className="relative ml-[17px] flex flex-col gap-px border-l border-border-strong pl-1.5" aria-label="Chats in this space">
      {rows.map((c) => (
        <li key={c.id}>
          <Link
            href={`/chat/${c.id}`}
            aria-current={c.id === chatId ? "page" : undefined}
            className={cx(
              "flex h-[26px] items-center truncate rounded-md px-2 text-[12.5px]",
              c.id === chatId ? "bg-accent-soft text-fg" : "text-fg-secondary hover:bg-hover hover:text-fg",
            )}
          >
            <span className="truncate">{chatTitle(c)}</span>
          </Link>
        </li>
      ))}
      <li>
        <Link href={`/spaces/${spaceId}`} className="flex h-[26px] items-center rounded-md px-2 text-xs text-fg-tertiary hover:bg-hover hover:text-fg">
          See all {total}
        </Link>
      </li>
    </ul>
  );
}

/** Chat mode's Spaces section (mockups 1 and 2): every space, with the open chat's space showing its newest chats. */
export function SidebarSpaces({ viewed }: { viewed: string | null }) {
  const spaces = useSpaces();
  const open = useOpenSpace(viewed);
  const [creating, setCreating] = useState(false);
  const list = spaces.data?.spaces ?? [];
  return (
    <section aria-label="Spaces" className="flex flex-col gap-px">
      <div className="flex items-center pb-1 pl-2 pr-1">
        <span className="flex-1 text-xs font-medium text-fg-tertiary">Spaces</span>
        <button
          type="button"
          aria-label="New space"
          title="New space"
          onClick={() => setCreating(true)}
          className="grid size-5 place-items-center rounded text-fg-secondary hover:bg-hover hover:text-fg"
        >
          <Icon name="plus" size={13} strokeWidth={1.8} />
        </button>
      </div>
      {spaces.isError ? (
        <p role="alert" className="px-2 text-xs text-pill-blocked-fg">Couldn’t load spaces.</p>
      ) : spaces.data && list.length === 0 ? (
        <button type="button" onClick={() => setCreating(true)} className="flex h-[30px] items-center gap-2.5 rounded-md px-2 text-left text-[13px] text-fg-tertiary hover:bg-hover hover:text-fg">
          <span className="grid size-4 place-items-center rounded border border-dashed border-border-strong">
            <Icon name="plus" size={10} />
          </span>
          New space
        </button>
      ) : (
        list.map((s) => {
          const expanded = !!open.chatSpace && open.chatSpace === s.id && !open.page;
          const active = open.page === s.id;
          return (
            <div key={s.id} className="flex flex-col gap-px">
              <Link
                href={`/spaces/${s.id}`}
                aria-current={active ? "page" : undefined}
                aria-expanded={expanded || undefined}
                className={cx(
                  "flex h-[28px] items-center gap-2.5 rounded-md px-2 text-[13px]",
                  active ? "bg-accent-soft text-fg" : expanded ? "text-fg hover:bg-hover" : "text-fg-secondary hover:bg-hover hover:text-fg",
                )}
              >
                <SpaceTile name={s.name} color={s.color} />
                <span className="min-w-0 flex-1 truncate">{s.name}</span>
                {expanded ? (
                  <Icon name="chev" size={11} className="flex-none text-fg-tertiary" />
                ) : (
                  <span className="flex-none text-[11px] text-fg-tertiary">{s.chat_count}</span>
                )}
              </Link>
              {expanded && <NestedChats spaceId={s.id} chatId={open.chatId} total={s.chat_count} />}
            </div>
          );
        })
      )}
      <NewSpaceDialog open={creating} onOpenChange={setCreating} />
    </section>
  );
}

/** The collapsed menu's spaces: one tile each, stacked under the nav icons. */
export function SpaceRail({ viewed }: { viewed: string | null }) {
  const spaces = useSpaces();
  const open = useOpenSpace(viewed);
  const list = spaces.data?.spaces ?? [];
  if (list.length === 0) return null;
  return (
    <nav aria-label="Spaces" className="flex w-full flex-col items-center gap-1 border-t border-border-subtle pt-2.5">
      {list.map((s) => {
        const on = open.page === s.id || (!open.page && open.chatSpace === s.id);
        return (
          <Link
            key={s.id}
            href={`/spaces/${s.id}`}
            aria-label={`${s.name} space`}
            title={s.name}
            aria-current={open.page === s.id ? "page" : undefined}
            className={cx("grid size-[30px] place-items-center rounded-md", on ? "bg-accent-soft" : "hover:bg-hover")}
          >
            <SpaceTile name={s.name} color={s.color} size={20} />
          </Link>
        );
      })}
    </nav>
  );
}
