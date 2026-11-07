"use client";

import { DropdownMenu } from "radix-ui";
import { useSyncExternalStore } from "react";
import type { Schemas } from "@/lib/api/client";
import { cx } from "@/lib/cx";
import { Icon } from "@/components/ui/icon";
import { AgentAvatar } from "@/components/domain/avatars";
import { floatPanel } from "@/components/domain/pickers";

type Agent = Schemas["Agent"];

// The agent picked for new chats (ADR open question 1: the person picks a main agent).
const STORAGE_KEY = "lazyagents.chat.agent";
const CHANGED = "la:chat-agent";

function subscribe(onChange: () => void) {
  window.addEventListener("storage", onChange);
  window.addEventListener(CHANGED, onChange);
  return () => {
    window.removeEventListener("storage", onChange);
    window.removeEventListener(CHANGED, onChange);
  };
}

// Fallback when storage is unavailable (private mode), so a pick still applies until reload.
let picked: string | null = null;

function read(): string | null {
  try {
    return window.localStorage.getItem(STORAGE_KEY) ?? picked;
  } catch {
    return picked;
  }
}

/** The agent id last used for a new chat; null before one is picked or on the server. */
export function useRememberedChatAgent(): string | null {
  return useSyncExternalStore(subscribe, read, () => null);
}

export function rememberChatAgent(id: string) {
  picked = id;
  try {
    window.localStorage.setItem(STORAGE_KEY, id);
  } catch {
    /* Storage unavailable: `picked` keeps it for this page. */
  }
  window.dispatchEvent(new Event(CHANGED));
}

const item =
  "flex min-h-10 cursor-default select-none items-center gap-2.5 rounded-md px-2 py-1 outline-none data-[highlighted]:bg-hover";

/** Main agent a new chat answers as. Fixed once the chat exists. */
export function AgentPicker({
  agents,
  value,
  onChange,
  disabled,
}: {
  agents: Agent[];
  value: Agent;
  onChange: (a: Agent) => void;
  disabled?: boolean;
}) {
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button
          type="button"
          disabled={disabled}
          aria-label={`Agent for new chats: ${value.name}`}
          className="flex h-7 min-w-0 flex-1 items-center gap-2 rounded-md border border-border-strong bg-btn-outline px-2 text-[13px] text-fg-2 hover:bg-hover disabled:cursor-not-allowed disabled:opacity-50"
        >
          <AgentAvatar name={value.name} colorIndex={value.color_index} label={false} />
          <span className="min-w-0 flex-1 truncate text-left">{value.name}</span>
          <Icon name="chev" size={13} strokeWidth={1.6} className="text-fg-tertiary" />
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align="start" sideOffset={6} className={cx(floatPanel, "w-[280px]")}>
          <DropdownMenu.Label className="px-2 py-1 text-xs font-medium text-fg-tertiary">Main agents</DropdownMenu.Label>
          {agents.map((a) => (
            <DropdownMenu.Item key={a.id} aria-label={a.name} className={item} onSelect={() => onChange(a)}>
              <AgentAvatar name={a.name} colorIndex={a.color_index} size="md" label={false} />
              <span className="flex min-w-0 flex-1 flex-col leading-tight">
                <span className="font-medium">{a.name}</span>
                {(a.specialty || a.role) && <span className="truncate text-[11.5px] text-fg-tertiary">{a.specialty || a.role}</span>}
              </span>
              <span className="w-4 text-accent">{a.id === value.id && <Icon name="check" size={14} strokeWidth={1.8} />}</span>
            </DropdownMenu.Item>
          ))}
          <DropdownMenu.Separator className="my-1 h-px bg-border-subtle" />
          <p className="px-2 py-1 text-[11.5px] text-fg-tertiary">
            A chat answers as this agent, read-only. It can’t be changed after the chat starts.
          </p>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}
