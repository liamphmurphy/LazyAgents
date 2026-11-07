"use client";

import Link from "next/link";
import { useState, type ReactNode } from "react";
import type { Artifact } from "@/lib/api/client";
import { useTicket } from "@/lib/api/hooks";
import { cx } from "@/lib/cx";
import { absoluteTime, timelineTime } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { TextArea } from "@/components/ui/primitives";
import { HumanAvatar } from "@/components/domain/avatars";
import { ReviewChip } from "@/components/domain/mockup";
import type { Pin } from "./review-canvas";

type Version = Artifact["versions"][number];
export type Decision = "approved" | "changes_requested";

function Section({ title, aside, children, className }: { title: string; aside?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <section aria-label={title} className={cx("flex flex-col gap-2 border-b border-border-subtle px-5 py-4", className)}>
      <div className="flex items-center gap-2">
        <h2 className="type-micro uppercase text-fg-tertiary">{title}</h2>
        {aside && <span className="ml-auto text-[11.5px] text-fg-tertiary">{aside}</span>}
      </div>
      {children}
    </section>
  );
}

function PinBadge({ n, tone }: { n: number; tone: "draft" | "saved" }) {
  return (
    <span className={cx("grid size-5 flex-none place-items-center rounded-full text-[11px] font-bold", tone === "draft" ? "bg-accent text-on-accent" : "bg-fg-secondary text-surface")}>
      {n}
    </span>
  );
}

/** The ticket's acceptance criteria, ticked locally as the reviewer checks the drawing. */
function Criteria({ ticketKey, keys, onTicket }: { ticketKey: string | null; keys: string[]; onTicket: (key: string) => void }) {
  const ticket = useTicket(ticketKey ?? "");
  const [ticked, setTicked] = useState<Record<string, boolean>>({});
  if (!ticketKey) {
    return (
      <Section title="Acceptance criteria">
        <p className="text-xs text-fg-tertiary">This mockup isn&apos;t attached to a ticket, so there are no criteria to check it against.</p>
      </Section>
    );
  }
  const criteria = ticket.data?.criteria ?? [];
  return (
    <Section
      title="Acceptance criteria"
      aside={
        keys.length > 1 ? (
          <select aria-label="Ticket" value={ticketKey} onChange={(e) => onTicket(e.target.value)} className="rounded-sm border border-border bg-surface px-1 font-mono text-[11.5px] text-fg-secondary">
            {keys.map((k) => (
              <option key={k} value={k}>
                {k}
              </option>
            ))}
          </select>
        ) : (
          <Link href={`/tickets/${ticketKey}`} className="font-mono text-accent hover:underline">
            {ticketKey} ↗
          </Link>
        )
      }
    >
      {ticket.isLoading ? (
        <p className="text-xs text-fg-tertiary">Loading criteria…</p>
      ) : ticket.isError ? (
        <p role="alert" className="text-xs text-pill-blocked-fg">Couldn&apos;t load {ticketKey}.</p>
      ) : criteria.length ? (
        <>
          <ul className="flex flex-col gap-1.5">
            {criteria.map((c) => (
              <li key={c.id}>
                <label className="flex cursor-pointer items-start gap-2.5 text-[12.5px] leading-snug">
                  <input
                    type="checkbox"
                    checked={!!ticked[c.id]}
                    onChange={(e) => setTicked((s) => ({ ...s, [c.id]: e.target.checked }))}
                    className="mt-0.5 size-3.5 flex-none accent-[var(--accent)]"
                  />
                  <span className={cx(ticked[c.id] ? "text-fg-secondary" : "text-fg")}>{c.text}</span>
                </label>
              </li>
            ))}
          </ul>
          <p className="text-[11px] text-fg-tertiary">Ticks are your checklist for this review; they aren&apos;t saved.</p>
        </>
      ) : (
        <p className="text-xs text-fg-tertiary">{ticketKey} has no acceptance criteria.</p>
      )}
    </Section>
  );
}

export function ReviewPanel({
  a,
  version,
  editable,
  pins,
  onRemovePin,
  activePin,
  onHoverPin,
  note,
  onNote,
  onDecide,
  deciding,
  error,
  saved,
  ticketKey,
  ticketKeys,
  onTicket,
  onPickVersion,
}: {
  a: Artifact;
  version: Version;
  editable: boolean;
  pins: Pin[];
  onRemovePin: (i: number) => void;
  activePin: number | null;
  onHoverPin: (i: number | null) => void;
  note: string;
  onNote: (s: string) => void;
  onDecide: (d: Decision) => void;
  deciding: boolean;
  error: string | null;
  saved: Decision | null;
  ticketKey: string | null;
  ticketKeys: string[];
  onTicket: (key: string) => void;
  onPickVersion: (id: string) => void;
}) {
  const review = version.review;
  const state = review?.state ?? "pending";
  const tone = editable ? "draft" : "saved";
  const others = a.versions.filter((v) => v.id !== version.id);
  return (
    <aside aria-label="Review" className="flex w-[360px] flex-none flex-col overflow-y-auto border-l border-border bg-surface max-lg:w-[300px]">
      <Section title="Reviewing">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-mono text-base font-semibold">{version.label}</span>
          <ReviewChip state={state} />
          {version.id === a.current_version.id && <span className="text-[11.5px] text-pill-done-fg">current</span>}
        </div>
        <p className="text-xs text-fg-secondary">
          {version.created_by.name} · <time dateTime={version.created_at} title={absoluteTime(version.created_at)}>{timelineTime(version.created_at)}</time>
          {version.note && <> · “{version.note}”</>}
        </p>
        {review?.reviewed_by && (
          <p className="flex items-center gap-1.5 text-xs text-fg-secondary">
            <HumanAvatar name={review.reviewed_by.name} initials={review.reviewed_by.initials} color={review.reviewed_by.avatar_color} size={16} label={false} />
            {review.reviewed_by.name}
            {review.reviewed_at && <time dateTime={review.reviewed_at} title={absoluteTime(review.reviewed_at)} className="text-fg-tertiary">· {timelineTime(review.reviewed_at)}</time>}
          </p>
        )}
        {!editable && review?.note && <p className="whitespace-pre-line text-[12.5px] text-fg-2">{review.note}</p>}
      </Section>

      <Criteria key={ticketKey} ticketKey={ticketKey} keys={ticketKeys} onTicket={onTicket} />

      <Section title="Pinned comments" aside={pins.length ? `${pins.length} on ${version.label}` : undefined}>
        {pins.length ? (
          <ol className="flex flex-col gap-1">
            {pins.map((p, i) => (
              <li
                key={i}
                onMouseEnter={() => onHoverPin(i)}
                onMouseLeave={() => onHoverPin(null)}
                className={cx("-mx-1.5 flex items-start gap-2 rounded-md px-1.5 py-1", activePin === i && "bg-hover")}
              >
                <PinBadge n={i + 1} tone={tone} />
                <span className="min-w-0 flex-1 whitespace-pre-line break-words text-[12.5px] leading-5">{p.text}</span>
                {editable && (
                  <button
                    type="button"
                    aria-label={`Remove pin ${i + 1}`}
                    title="Remove"
                    onClick={() => onRemovePin(i)}
                    onFocus={() => onHoverPin(i)}
                    onBlur={() => onHoverPin(null)}
                    className="grid size-5 flex-none place-items-center rounded-sm text-fg-tertiary hover:bg-hover hover:text-fg"
                  >
                    ×
                  </button>
                )}
              </li>
            ))}
          </ol>
        ) : (
          <p className="text-xs text-fg-tertiary">{editable ? "No pins yet." : `No pins on ${version.label}.`}</p>
        )}
        {editable && <p className="text-[11px] text-fg-tertiary">Press C or choose Pin, then click the mockup. Pins are added to the note as a numbered list.</p>}
      </Section>

      {editable ? (
        <Section title="Note" aside="Required to request changes">
          <TextArea
            id="review-note"
            aria-label="Note"
            rows={4}
            maxLength={2000}
            value={note}
            onChange={(e) => onNote(e.target.value)}
            placeholder="Overall feedback for the agent…"
            className="text-[13px]"
          />
          <p className="text-[11px] text-fg-tertiary">Sent with the decision; pins follow as “1. … 2. …”.</p>
          <div className="mt-1 flex gap-2">
            <Button variant="approve-outline" className="h-9 flex-1 justify-center" disabled={!note.trim() || deciding} onClick={() => onDecide("changes_requested")}>
              Request changes
            </Button>
            <Button variant="approve" icon="check" className="h-9 flex-1 justify-center" disabled={state === "approved" || deciding} onClick={() => onDecide("approved")}>
              Approve {version.label}
            </Button>
          </div>
          <p className="text-[11px] text-fg-tertiary">Request changes needs a note. Agents waiting on this mockup resume with your decision; the ticket&apos;s gate updates.</p>
          {error && (
            <p role="alert" className="text-xs text-pill-blocked-fg">
              {error}
            </p>
          )}
          <p role="status" className="text-xs text-pill-done-fg empty:hidden">
            {saved === "approved" ? `Approved ${version.label}.` : saved === "changes_requested" ? `Changes requested on ${version.label}.` : ""}
          </p>
        </Section>
      ) : (
        <Section title="Decision">
          <p className="text-xs text-fg-tertiary">Only the current version can be reviewed.</p>
          <Button variant="outline" className="self-start" onClick={() => onPickVersion(a.current_version.id)}>
            Review {a.current_version.label}
          </Button>
        </Section>
      )}

      {others.length > 0 && (
        <Section title="History" className="border-b-0">
          <ul className="flex flex-col">
            {others.map((v) => (
              <li key={v.id}>
                <button
                  type="button"
                  onClick={() => onPickVersion(v.id)}
                  className="-mx-1.5 flex w-[calc(100%+12px)] items-center gap-2 rounded-md px-1.5 py-1 text-left text-xs hover:bg-hover"
                >
                  <span className="font-mono text-[12px] font-semibold">{v.label}</span>
                  {v.review && <ReviewChip state={v.review.state} />}
                  {v.review && v.review.state !== "pending" && (
                    <span className="text-fg-secondary">{v.review.pins.length ? `${v.review.pins.length} pin${v.review.pins.length === 1 ? "" : "s"}` : v.review.note ? "note only" : ""}</span>
                  )}
                  <span className="ml-auto truncate text-fg-tertiary">
                    {v.review?.reviewed_by ? `${v.review.reviewed_by.name} · ` : ""}
                    {timelineTime(v.review?.reviewed_at ?? v.created_at)}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </Section>
      )}
    </aside>
  );
}
