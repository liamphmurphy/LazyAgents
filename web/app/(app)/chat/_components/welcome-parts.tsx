"use client";

import Link from "next/link";
import { useChats, useInbox, usePersonaDock } from "@/lib/api/hooks";
import { usePersonas } from "@/lib/api/persona-hooks";
import { cx } from "@/lib/cx";
import { timelineTime } from "@/lib/format";
import { toneLabel } from "@/lib/persona-ui";
import { carouselOrder, carouselWindow, personaTagline, pickUpCards, type PickUpTone } from "@/lib/welcome";
import { PersonaCarousel, personaLook } from "./persona";

// The persona-led welcome's parts, shared by the welcome (/chat?new) and an empty conversation.

/** Who you're talking to: the carousel, the persona's name and its tagline. Without onPick there's nothing to switch, so only the portrait shows. */
export function PersonaHero({ personaId, onPick, disabled, level }: { personaId?: string; onPick?: (id: string) => void; disabled?: boolean; level: 1 | 2 }) {
  const personas = usePersonas();
  const defaultId = personas.data?.default_persona_id;
  const full = personas.data?.personas.find((p) => p.id === personaId);
  const persona = personaLook(full);
  const ordered = onPick ? carouselOrder(personas.data?.personas ?? [], defaultId) : full ? [full] : [];
  const around = carouselWindow(ordered, ordered.findIndex((p) => p.id === personaId));
  const Heading = level === 1 ? "h1" : "h2";
  return (
    <div className="flex flex-col items-center text-center">
      <PersonaCarousel ordered={ordered} current={persona} left={around.left} right={around.right} onPick={(id) => onPick?.(id)} disabled={disabled || !onPick} />
      <Heading className="mt-5 text-[26px] font-semibold leading-tight tracking-[-0.02em] text-fg">{persona.name}</Heading>
      <p className="mt-2 text-[14px] text-fg-secondary">{personaTagline(full, toneLabel, !!full && full.id === defaultId)}</p>
    </div>
  );
}

const PICK_UP_PILL: Record<PickUpTone, string> = {
  human: "bg-pill-human-bg text-pill-human-fg",
  persona: "",
  neutral: "bg-pill-neutral-bg text-pill-neutral-fg",
};

/** "Pick up where you left off": what needs you, unread replies, then your latest chats. Nothing until there's something. */
export function PickUp() {
  const inbox = useInbox();
  const dock = usePersonaDock();
  const chats = useChats(false, "all");
  const cards = pickUpCards(inbox.data ?? [], dock.data?.personas ?? [], chats.data?.chats ?? [], (iso) => timelineTime(iso));
  if (!cards.length) return null;
  return (
    <section aria-label="Pick up where you left off" className="flex w-full flex-col gap-3">
      <h2 className="text-xs font-semibold uppercase tracking-[0.04em] text-fg-tertiary">Pick up where you left off</h2>
      <ul className="grid grid-cols-3 gap-3 max-md:grid-cols-1">
        {cards.map((c) => (
          <li key={c.key} className="min-w-0">
            <Link href={c.href} className="flex h-full flex-col items-start gap-2 rounded-xl border border-border bg-surface p-3.5 text-left hover:border-border-hover hover:bg-hover">
              <span
                className={cx("rounded-full px-2.5 py-0.5 text-xs font-semibold", PICK_UP_PILL[c.tone])}
                style={c.tone === "persona" && c.color ? { color: c.color, background: `color-mix(in srgb, ${c.color} 15%, transparent)` } : undefined}
              >
                {c.label}
              </span>
              <span className="line-clamp-2 text-[13.5px] font-medium text-fg">{c.title}</span>
              <span className="truncate text-xs text-fg-tertiary">{c.meta}</span>
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
}
