"use client";

import { useChatSuggestions } from "@/lib/api/hooks";
import { Icon } from "@/components/ui/icon";

/** Four conversation starters under an empty chat's greeting. Picking one sends its prompt. */
export function ChatSuggestions({ onPick, disabled }: { onPick: (prompt: string) => void; disabled: boolean }) {
  const suggestions = useChatSuggestions().data?.suggestions;
  if (suggestions?.length !== 4) return null;
  return (
    <ul aria-label="Conversation starters" className="mx-auto grid w-full max-w-[560px] grid-cols-2 gap-2">
      {suggestions.map((s) => (
        <li key={s.title} className="min-w-0">
          <button
            type="button"
            disabled={disabled}
            title={s.prompt}
            onClick={() => onPick(s.prompt)}
            className="flex h-full w-full items-start gap-2.5 rounded-md border border-border bg-surface-raised p-3 text-left hover:bg-hover disabled:opacity-60"
          >
            <Icon name={s.icon} size={18} className="mt-px flex-none text-fg-secondary" />
            <span className="min-w-0">
              <span className="block truncate text-13 font-medium text-fg">{s.title}</span>
              <span className="line-clamp-2 block text-12 text-fg-secondary">{s.prompt}</span>
            </span>
          </button>
        </li>
      ))}
    </ul>
  );
}
