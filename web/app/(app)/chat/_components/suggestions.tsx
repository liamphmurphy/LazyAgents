"use client";

import { useChatSuggestions } from "@/lib/api/hooks";
import { Icon } from "@/components/ui/icon";

/** Four conversation starters under an empty chat's persona, as chips. Picking one sends its prompt. */
export function ChatSuggestions({ onPick, disabled, color }: { onPick: (prompt: string) => void; disabled: boolean; color?: string }) {
  const suggestions = useChatSuggestions().data?.suggestions;
  if (suggestions?.length !== 4) return null;
  return (
    <ul aria-label="Conversation starters" className="flex max-w-[768px] flex-wrap justify-center gap-2.5">
      {suggestions.map((s) => (
        <li key={s.title} className="min-w-0">
          <button
            type="button"
            disabled={disabled}
            title={s.prompt}
            onClick={() => onPick(s.prompt)}
            className="inline-flex h-8 max-w-full items-center gap-2 rounded-full border border-border bg-surface px-3.5 text-[13px] text-fg-2 hover:bg-hover hover:text-fg disabled:opacity-60"
          >
            <span className="flex-none" style={color ? { color } : undefined}><Icon name={s.icon} size={14} /></span>
            <span className="truncate">{s.title}</span>
          </button>
        </li>
      ))}
    </ul>
  );
}
