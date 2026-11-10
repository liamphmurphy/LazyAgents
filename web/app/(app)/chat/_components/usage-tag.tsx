"use client";

import { Tooltip } from "radix-ui";
import type { ReactNode } from "react";
import type { Schemas } from "@/lib/api/client";
import { compact } from "@/lib/format";
import { replyCost } from "@/lib/chat-ui";
import { tokenBreakdown } from "@/lib/token-usage";
import { cx } from "@/lib/cx";
import { floatPanel } from "@/components/domain/pickers";
import { effortLabel } from "./model-picker";

type Message = Schemas["ChatMessage"];

function Row({ label, value, note }: { label: string; value: string; note?: string }) {
  return (
    <div className="flex items-baseline gap-2">
      <span className="w-14 text-fg-secondary">{label}</span>
      <span className="font-mono">{value}</span>
      {note && <span className="text-fg-tertiary">{note}</span>}
    </div>
  );
}

const REASONS: Record<NonNullable<Message["effort_reasons"]>[number], string> = {
  confirmation: "raised for a go-ahead",
  effort_mismatch: "raised after a retry or edit",
};

/** How the reply's effort was chosen: by the person, or by Vesper and why. */
function effortNote(m: Message): string | undefined {
  if (!m.effort_level) return m.effort && "chosen";
  const aimed = m.effort_level !== m.effort && `aimed for ${effortLabel(m.effort_level).toLowerCase()}`;
  return ["automatic", aimed, ...(m.effort_reasons ?? []).map((r) => REASONS[r])].filter(Boolean).join(" · ");
}

/**
 * The reply's harness · model · effort tag. When the turn reported usage, the
 * tag is a button whose tooltip (hover or keyboard focus) shows tokens, cost
 * and how the effort was chosen.
 */
export function UsageTag({ m, children }: { m: Message; children: ReactNode }) {
  const tagClass = "font-mono text-xs text-fg-tertiary";
  if (m.input_tokens === undefined) {
    return (
      <span className={tagClass} data-testid="model-tag">
        {children}
      </span>
    );
  }
  const t = tokenBreakdown(m);
  const cost = replyCost(m.cost_usd);
  const cache = [
    t.hasBreakdown && `${compact(t.cached)} cached · ${compact(t.uncached)} uncached`,
    t.unknown > 0 && `${compact(t.unknown)} cache unknown`,
  ]
    .filter(Boolean)
    .join(" · ");
  return (
    <>
      {/* The Tooltip.Provider is in Conversation, shared by every reply. */}
      <Tooltip.Root>
        <Tooltip.Trigger asChild>
          <button
            type="button"
            data-testid="model-tag"
            className={cx(tagClass, "cursor-help rounded-xs underline decoration-dotted underline-offset-2 hover:text-fg-secondary focus-visible:outline focus-visible:outline-accent")}
          >
            {children}
          </button>
        </Tooltip.Trigger>
        <Tooltip.Portal>
          <Tooltip.Content
            side="top"
            align="start"
            sideOffset={6}
            data-testid="usage-tooltip"
            className={cx(floatPanel, "flex flex-col gap-1 px-3 py-2 text-xs")}
          >
            <span className="text-xs text-fg-tertiary">This reply</span>
            <Row label="Input" value={compact(t.input)} note={cache || undefined} />
            <Row label="Output" value={compact(t.output)} />
            <Row label="Cost" value={cost ?? "not reported"} />
            {(m.effort || m.effort_level) && <Row label="Effort" value={m.effort ? effortLabel(m.effort) : "Model default"} note={effortNote(m)} />}
          </Tooltip.Content>
        </Tooltip.Portal>
      </Tooltip.Root>
      {cost && <span className="text-xs text-fg-tertiary"> · {cost}</span>}
    </>
  );
}
