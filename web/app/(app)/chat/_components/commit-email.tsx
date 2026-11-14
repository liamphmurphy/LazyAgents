"use client";

import { previewHash } from "@/lib/commit-email";
import { repoShortName } from "@/lib/persona-ui";
import { cx } from "@/lib/cx";
import { Icon } from "@/components/ui/icon";

/**
 * "Commit as": each of your addresses drawn as the commit it would author, on
 * a little git graph. Shown when a coding chat could commit as more than one
 * of your addresses; picking one sets the author from the next turn.
 */
export function CommitEmailPicker({
  emails,
  name,
  repo,
  value,
  onPick,
  onClose,
  disabled,
  required,
}: {
  emails: string[];
  name: string;
  repo: string;
  value?: string;
  onPick: (email: string) => void;
  /** Offered once an address is picked, to fold the picker back into its chip. */
  onClose?: () => void;
  disabled?: boolean;
  /** Before the first message: sending waits for a pick. */
  required?: boolean;
}) {
  const picked = value?.toLowerCase();
  return (
    <section aria-labelledby="commit-email-title" data-testid="commit-email-picker" className="rounded-xl border border-border bg-surface p-3 shadow-sm">
      <div className="mb-2.5 flex items-start gap-2.5">
        <CommitGlyph />
        <div className="min-w-0 flex-1">
          <h2 id="commit-email-title" className="text-13 font-semibold text-fg">Which email should commits use?</h2>
          <p className="text-12 text-fg-secondary">
            Commits in <span className="font-mono text-12 text-fg">{repoShortName(repo)}</span> are authored as you.
            {required ? " Pick one to start; you can change it later." : " It applies from the next reply."}
          </p>
        </div>
        {onClose && value && (
          <button type="button" aria-label="Close" className="flex-none rounded p-1 text-fg-tertiary hover:bg-hover hover:text-fg" onClick={onClose}>
            <Icon name="x" size={12} />
          </button>
        )}
      </div>
      <div role="radiogroup" aria-labelledby="commit-email-title" className="grid gap-2 sm:grid-cols-2">
        {emails.map((email, i) => {
          const on = email.toLowerCase() === picked;
          return (
            <button
              key={email}
              type="button"
              role="radio"
              aria-checked={on}
              disabled={disabled}
              onClick={() => onPick(email)}
              className={cx(
                "group flex min-w-0 items-stretch gap-2.5 rounded-lg border px-2.5 py-2 text-left transition-colors disabled:cursor-not-allowed disabled:opacity-60",
                on ? "border-accent bg-selected" : "border-border hover:border-accent hover:bg-hover",
              )}
            >
              <GraphRail on={on} />
              <span className="flex min-w-0 flex-1 flex-col gap-0.5 font-mono text-12 leading-snug">
                <span className="flex items-center gap-1.5">
                  <span className={on ? "text-accent" : "text-pill-human-fg"}>commit {previewHash(email)}</span>
                  {i === 0 && <span className="rounded-full bg-hover px-1.5 font-sans text-11 text-fg-secondary">primary</span>}
                </span>
                <span className="truncate text-fg-secondary" title={`${name} <${email}>`}>
                  Author: {name} &lt;<span className="text-fg">{email}</span>&gt;
                </span>
              </span>
              <span
                aria-hidden
                className={cx(
                  "flex size-4 flex-none items-center justify-center self-center rounded-full border",
                  on ? "border-accent bg-accent text-on-accent" : "border-border-strong",
                )}
              >
                {on && <Icon name="check" size={10} />}
              </span>
            </button>
          );
        })}
      </div>
    </section>
  );
}

/** A commit dot on a branch line: the commit this address would author. */
function GraphRail({ on }: { on: boolean }) {
  return (
    <svg aria-hidden width="14" height="34" viewBox="0 0 14 34" className={cx("flex-none self-center", on ? "text-accent" : "text-fg-tertiary group-hover:text-accent")}>
      <path d="M7 0v11M7 23v11" stroke="currentColor" strokeWidth="1.5" strokeDasharray={on ? undefined : "2 2"} />
      <circle cx="7" cy="17" r="5" fill={on ? "currentColor" : "none"} stroke="currentColor" strokeWidth="1.5" />
    </svg>
  );
}

/** Three commits on a branch, the newest highlighted. */
function CommitGlyph() {
  return (
    <svg aria-hidden width="30" height="30" viewBox="0 0 30 30" className="flex-none text-accent">
      <rect x="0.75" y="0.75" width="28.5" height="28.5" rx="8" fill="currentColor" fillOpacity="0.1" stroke="currentColor" strokeOpacity="0.35" />
      <path d="M7 15h16" stroke="currentColor" strokeWidth="1.5" strokeOpacity="0.5" />
      <circle cx="8" cy="15" r="2" fill="currentColor" fillOpacity="0.5" />
      <circle cx="15" cy="15" r="2" fill="currentColor" fillOpacity="0.5" />
      <circle cx="22" cy="15" r="3.2" fill="currentColor" />
    </svg>
  );
}
