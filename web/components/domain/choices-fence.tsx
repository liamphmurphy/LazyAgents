"use client";

import { useContext } from "react";
import { choicePrompt, chosenOption, parseChoices } from "@/lib/chat-choices";
import { cx } from "@/lib/cx";
import { Icon } from "@/components/ui/icon";
import { CodeFence } from "./code-fence";
import { RecommendActions } from "./recommend-fence";
import { SvgFenceMessage } from "./svg-fence";

/** A ```choices fence: options the person answers with a click. Once the chat moves on it only records the pick. */
export function ChoicesFence({ code, info }: { code: string; info?: string }) {
  const parsed = parseChoices(code);
  const messageId = useContext(SvgFenceMessage);
  const actions = useContext(RecommendActions);
  if (!parsed) return <CodeFence code={code} info={info} />;
  const next = messageId ? actions?.nextMessage(messageId) : undefined;
  const chosen = chosenOption(parsed.options, next);
  const closed = !messageId || !actions || actions.disabled || next !== undefined;
  return (
    <div data-testid="choices" role="group" aria-label={parsed.question || "Options"} className="flex w-[480px] max-w-full flex-col gap-2 rounded-xl border border-border bg-surface p-3">
      {parsed.question && <p className="text-14 font-semibold text-fg">{parsed.question}</p>}
      <div className="flex flex-col gap-1.5">
        {parsed.options.map((o) => (
          <button
            key={o.label}
            type="button"
            disabled={closed}
            aria-pressed={chosen === o.label}
            onClick={() => actions?.ask(messageId!, choicePrompt(o.label))}
            className={cx(
              "flex items-center gap-2.5 rounded-lg border px-3 py-2 text-left transition-colors",
              chosen === o.label ? "border-accent bg-accent-soft" : "border-border",
              !closed && "hover:border-border-strong hover:bg-hover",
              closed && chosen !== o.label && "opacity-55",
            )}
          >
            <span className="flex min-w-0 flex-1 flex-col">
              <span className="text-13 font-semibold text-fg">{o.label}</span>
              {o.detail && <span className="text-12 text-fg-secondary">{o.detail}</span>}
            </span>
            {chosen === o.label && <Icon name="check" size={14} className="flex-none text-accent" />}
          </button>
        ))}
      </div>
    </div>
  );
}
