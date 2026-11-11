"use client";

import { Icon, type IconName } from "@/components/ui/icon";
import { cx } from "@/lib/cx";
import { SEND_MODE_HINT, SEND_MODE_LABEL, SEND_MODES, type SendMode } from "@/lib/send-mode";
import type { SendTone } from "./reply-tools";

const ICON: Record<SendMode, IconName> = { followup: "clock", aside: "branch", interrupt: "bolt" };

const ON: Record<SendMode, string> = {
  followup: "bg-accent-soft text-accent",
  aside: "bg-pill-review-bg text-pill-review-fg",
  interrupt: "bg-pill-human-bg text-pill-human-fg",
};

export const SEND_TONE: Record<SendMode, SendTone> = { followup: "accent", aside: "aside", interrupt: "interrupt" };

/** While a reply runs: whether Send queues a follow-up, asks an aside or interrupts. */
export function SendModeSwitch({ value, onChange, disabled }: { value: SendMode; onChange: (mode: SendMode) => void; disabled?: boolean }) {
  return (
    <div role="radiogroup" aria-label="Send as" data-testid="send-modes" className="flex h-[30px] flex-none items-center gap-0.5 rounded-full border border-border p-0.5">
      {SEND_MODES.map((mode) => (
        <button
          key={mode}
          type="button"
          role="radio"
          aria-checked={value === mode}
          title={SEND_MODE_HINT[mode]}
          disabled={disabled}
          onClick={() => onChange(mode)}
          className={cx(
            "inline-flex h-[24px] items-center gap-1.5 rounded-full px-2.5 text-[12.5px] font-medium disabled:cursor-not-allowed disabled:opacity-50",
            value === mode ? ON[mode] : "text-fg-secondary hover:text-fg",
          )}
        >
          <Icon name={ICON[mode]} size={12} />
          <span className="max-sm:sr-only">{SEND_MODE_LABEL[mode]}</span>
        </button>
      ))}
    </div>
  );
}
