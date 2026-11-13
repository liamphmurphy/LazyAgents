"use client";

import Link from "next/link";
import { ApiError } from "@/lib/api/client";
import { useDecideTicketChange, useTicketChangeRequests, type TicketChangeDecision, type TicketChangeRequest } from "@/lib/api/ticket-change-hooks";
import { Button } from "@/components/ui/button";
import { PersonaAvatar, PersonaName, personaLook } from "./persona";

type Change = TicketChangeRequest["changes"][number];

const message = (e: unknown) => (e instanceof ApiError || e instanceof Error ? e.message : null);

const VERB: Record<TicketChangeRequest["tool"], string> = {
  update_ticket: "wants to change",
  comment_ticket: "wants to comment on",
  update_ticket_blockers: "wants to change what blocks",
};

/** Enum values (in_progress) read as words; everything else is shown as the server wrote it. */
const display = (c: Change, v?: string) => (v && (c.field === "status" || c.field === "priority") ? v.replaceAll("_", " ") : v);

/** The server writes criteria as Markdown task items; the card shows them as boxes. */
const blockText = (c: Change, v?: string) => (c.field === "criteria" && v ? v.replace(/^- \[x\] /gm, "☑ ").replace(/^- \[ \] /gm, "☐ ") : v) || "(empty)";

/** Text that wraps or runs long reads better stacked than as "before → after" on one line. */
const isBlock = (c: Change) => c.field === "description" || c.field === "criteria" || c.field === "comment" || `${c.before ?? ""}${c.after}`.includes("\n") || `${c.before ?? ""}${c.after}`.length > 80;

/**
 * Ticket edits a persona proposed in this chat. Nothing changes until the person applies one;
 * applying runs it as the persona, and a ticket that changed since is refused with why.
 */
export function TicketChangeRequests({ chatId, disabled }: { chatId: string; disabled?: boolean }) {
  const requests = useTicketChangeRequests(chatId);
  const decide = useDecideTicketChange(chatId);
  const waiting = requests.data?.requests ?? [];
  const error = message(decide.error);
  // A refused apply leaves no card behind, so its reason is kept on screen on its own.
  if (!waiting.length && !error) return null;
  const busy = decide.isPending || !!disabled;
  const onDecide = (id: string, decision: TicketChangeDecision) => decide.mutate({ id, decision });
  return (
    <li className="flex flex-col gap-2" data-testid="ticket-change-requests">
      {waiting.map((r) => (
        <ChangeCard key={r.id} r={r} busy={busy} onDecide={(d) => onDecide(r.id, d)} />
      ))}
      {error && (
        <p role="alert" className="max-w-[min(720px,100%)] text-[12.5px] text-pill-blocked-fg">
          Not applied: {error}
        </p>
      )}
    </li>
  );
}

function ChangeCard({ r, busy, onDecide }: { r: TicketChangeRequest; busy: boolean; onDecide: (d: TicketChangeDecision) => void }) {
  const by = personaLook(r.persona);
  const name = r.persona?.name ?? "The assistant";
  return (
    <div data-testid="ticket-change" className="flex max-w-[min(720px,100%)] flex-col gap-2 rounded-xl border border-border bg-surface px-3.5 py-3">
      <p className="flex flex-wrap items-center gap-1.5 text-xs text-fg-tertiary">
        <PersonaAvatar persona={by} size={16} />
        <PersonaName persona={by} />
        <span>{VERB[r.tool]}</span>
        <Link href={`/tickets/${r.ticket_key}`} className="font-mono text-accent hover:underline">
          {r.ticket_key}
        </Link>
      </p>
      <p className="text-[14px] font-medium text-fg">{r.ticket_title}</p>
      <dl className="flex flex-col gap-1.5 rounded-md border border-border-subtle bg-surface-sunken px-3 py-2">
        {r.changes.map((c) => (
          <ChangeRow key={c.field} c={c} />
        ))}
      </dl>
      <div className="flex flex-wrap justify-end gap-2">
        <Button variant="outline" disabled={busy} onClick={() => onDecide("deny")}>
          Decline
        </Button>
        <Button variant="outline" disabled={busy} onClick={() => onDecide("always")} title={`Apply this, and let ${name} edit tickets without asking from now on. Canceling a ticket still asks.`}>
          Always allow
        </Button>
        <Button variant="primary" disabled={busy} onClick={() => onDecide("allow")}>
          Apply
        </Button>
      </div>
    </div>
  );
}

function ChangeRow({ c }: { c: Change }) {
  if (!isBlock(c)) {
    return (
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-0.5 text-[13px]" data-testid="ticket-change-row">
        <dt className="w-32 shrink-0 text-fg-tertiary">{c.label}</dt>
        <dd className="flex min-w-0 flex-wrap items-baseline gap-1.5 break-words">
          {c.before !== undefined && (
            <>
              <span className="text-fg-tertiary line-through">{display(c, c.before)}</span>
              <span aria-label="becomes" className="text-fg-tertiary">→</span>
            </>
          )}
          <span className="text-fg">{display(c, c.after)}</span>
        </dd>
      </div>
    );
  }
  return (
    <div className="flex flex-col gap-1 text-[13px]" data-testid="ticket-change-row">
      <dt className="text-fg-tertiary">{c.label}</dt>
      <dd className="flex flex-col gap-1">
        {c.before !== undefined && (
          <details className="text-[12.5px]">
            <summary className="cursor-pointer text-fg-tertiary">Current</summary>
            <pre className="mt-1 max-h-40 overflow-auto whitespace-pre-wrap break-words font-sans text-fg-tertiary">{blockText(c, c.before)}</pre>
          </details>
        )}
        <pre className="max-h-60 overflow-auto whitespace-pre-wrap break-words font-sans text-fg">{blockText(c, c.after)}</pre>
      </dd>
    </div>
  );
}
