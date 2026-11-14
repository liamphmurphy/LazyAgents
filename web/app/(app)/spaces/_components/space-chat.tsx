"use client";

import Link from "next/link";
import { useState } from "react";
import type { Schemas } from "@/lib/api/client";
import { spaceHex } from "@/lib/spaces";
import { ChatProjectPicker } from "@/app/(app)/chat/_components/chat-project";
import { SpaceTile } from "./space-parts";
import { SourcesDialog } from "./sources-dialog";

type SpaceRef = Schemas["ChatSpaceRef"];

/** "Health /" ahead of a space chat's title (mockup 2). */
export function SpaceBreadcrumb({ space }: { space: SpaceRef }) {
  return (
    <span className="flex flex-none items-center gap-[8px] text-13">
      <Link href={`/spaces/${space.id}`} className="flex items-center gap-[8px] rounded-md text-fg-secondary hover:text-fg" title={`Open ${space.name}`}>
        <SpaceTile name={space.name} color={space.color} />
        <span className="max-w-[180px] truncate">{space.name}</span>
      </Link>
      <span aria-hidden className="text-fg-tertiary">/</span>
    </span>
  );
}

const chip = "inline-flex h-[26px] flex-none items-center rounded-full border px-[12px] text-12";

/** What the space gives this chat: its instructions, sources and memory, plus the project (mockup 2). */
/** `dense`: a narrow dock, where the chips sit tight under its header instead of centred over a page. */
export function SpaceContextChips({ space, chat, disabled, dense }: { space: SpaceRef; chat: { id: string; project?: string; revision: number; archived: boolean }; disabled: boolean; dense?: boolean }) {
  const [sources, setSources] = useState(false);
  const hex = spaceHex(space.color);
  return (
    <div className={dense ? "px-3 pt-2" : "px-6 pt-[16px]"}>
      <div aria-label={`From ${space.name}`} role="group" className="mx-auto flex max-w-[860px] flex-wrap items-center gap-[8px]">
        {space.has_instructions && (
          <Link
            href={`/spaces/${space.id}`}
            className={chip}
            title={`${space.name}’s instructions apply to this chat`}
            style={{
              background: `color-mix(in srgb, ${hex} 12%, transparent)`,
              borderColor: `color-mix(in srgb, ${hex} 40%, transparent)`,
              color: `color-mix(in srgb, ${hex} 55%, var(--text))`,
            }}
          >
            {space.name} instructions
          </Link>
        )}
        {space.source_count > 0 && (
          <button type="button" className={`${chip} border-border-strong bg-surface-raised text-fg-secondary hover:bg-hover`} onClick={() => setSources(true)}>
            {space.source_count} {space.source_count === 1 ? "source" : "sources"}
          </button>
        )}
        {space.space_memory && (
          <Link href={`/customization?space=${space.id}`} className={`${chip} border-border-strong bg-surface-raised text-fg-secondary hover:bg-hover`} title={`What’s learned here stays in ${space.name}`}>
            Space memory
          </Link>
        )}
        <ChatProjectPicker chat={chat} disabled={disabled} dashed />
      </div>
      <SourcesDialog spaceId={space.id} spaceName={space.name} open={sources} onOpenChange={setSources} />
    </div>
  );
}
