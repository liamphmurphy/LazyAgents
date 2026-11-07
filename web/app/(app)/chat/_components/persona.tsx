"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useSyncExternalStore, type CSSProperties } from "react";
import { DropdownMenu } from "radix-ui";
import type { Schemas } from "@/lib/api/client";
import { usePersonas } from "@/lib/api/hooks";
import { cx } from "@/lib/cx";
import { Icon } from "@/components/ui/icon";
import { floatPanel } from "@/components/domain/pickers";

// Assistant personas in the Assistant view only (ADR: Assistant personas). Ticket,
// epic and run views keep their agent identity and never render these.

type Persona = Schemas["Persona"];
type Summary = Schemas["PersonaSummary"];
/** What a chat needs to show who it is talking to; a null persona is the built-in Assistant. */
export type PersonaLook = { id?: string; name: string; color: string; avatar_url?: string };

/** The built-in Assistant: chats without a persona, or whose persona was deleted. */
export const BUILTIN: PersonaLook = { name: "Assistant", color: "var(--accent)" };

export const personaLook = (p?: Summary | Persona | null): PersonaLook => p ?? BUILTIN;

const TONE: Record<Schemas["PersonaTone"], string> = {
  neutral: "Neutral",
  friendly: "Friendly",
  concise: "Concise",
  formal: "Formal",
  playful: "Playful",
};
export const toneLabel = (t?: Schemas["PersonaTone"]) => (t ? TONE[t] : "Neutral");

/** Avatar image cropped to a circle, or the initial on a tint of the persona color. */
export function PersonaAvatar({ persona, size = 20, className }: { persona: PersonaLook; size?: number; className?: string }) {
  const style: CSSProperties = { width: size, height: size };
  if (persona.avatar_url) {
    return (
      // A same-origin, private, cache-busted URL: next/image would only add a proxy hop.
      // eslint-disable-next-line @next/next/no-img-element
      <img src={persona.avatar_url} alt="" aria-hidden className={cx("flex-none rounded-full object-cover", className)} style={style} />
    );
  }
  return (
    <span
      aria-hidden
      className={cx("grid flex-none place-items-center rounded-full font-semibold", className)}
      style={{
        ...style,
        fontSize: Math.round(size * 0.5),
        color: persona.color,
        background: `color-mix(in srgb, ${persona.color} 15%, transparent)`,
      }}
    >
      {persona.name.trim().slice(0, 1).toUpperCase() || "A"}
    </span>
  );
}

/** The persona's name in its color. */
export function PersonaName({ persona, className }: { persona: PersonaLook; className?: string }) {
  return (
    <span className={cx("font-semibold", className)} style={{ color: persona.color }}>
      {persona.name}
    </span>
  );
}

// ---- Sidebar filter: kept for the browser session, reset when the default changes.

/** "default" (the server's default), a persona id, or "all". */
export type FilterValue = string;
type Stored = { v: FilterValue; d: string };

const STORE_KEY = "lazyagents.assistant.persona-filter";
const listeners = new Set<() => void>();
function readStored(): string | null {
  try {
    return sessionStorage.getItem(STORE_KEY);
  } catch {
    return null;
  }
}
function subscribe(fn: () => void) {
  listeners.add(fn);
  return () => listeners.delete(fn);
}
function writeStored(s: Stored) {
  try {
    sessionStorage.setItem(STORE_KEY, JSON.stringify(s));
  } catch {
    /* Private mode: the choice lasts until the next render only. */
  }
  for (const fn of listeners) fn();
}
function parseStored(raw: string | null): Stored | null {
  if (!raw) return null;
  try {
    const s = JSON.parse(raw) as Stored;
    return typeof s?.v === "string" && typeof s?.d === "string" ? s : null;
  } catch {
    return null;
  }
}

export type PersonaFilterState = {
  /** The dropdown's value. */
  value: FilterValue;
  set: (v: FilterValue) => void;
  /** persona_id for GET /chats: "" lets the server use the default persona. */
  query: string;
  personas: Persona[];
  /** The default persona, or the built-in Assistant when there's none. */
  defaultLook: PersonaLook;
  defaultId?: string;
  error: unknown;
  loading: boolean;
  retry: () => void;
};

/**
 * The sidebar's persona filter. It starts on the default persona; a pick lasts
 * for the browser session until the default changes. If personas can't load,
 * the list falls back to every conversation.
 */
export function usePersonaFilter(): PersonaFilterState {
  const q = usePersonas();
  const raw = useSyncExternalStore(subscribe, readStored, () => null);
  const personas = q.data?.personas ?? [];
  const defaultId = q.data?.default_persona_id;
  const defaultPersona = personas.find((p) => p.id === defaultId);
  const stored = parseStored(raw);
  // A stored pick made against another default, or for a persona that's gone, resets to the default.
  let value: FilterValue = "default";
  if (stored && q.data && stored.d === (defaultId ?? "")) {
    if (stored.v === "all" || personas.some((p) => p.id === stored.v && p.id !== defaultId)) value = stored.v;
  }
  const query = q.isError ? "all" : value === "default" ? "" : value;
  return {
    value: q.isError ? "all" : value,
    set: (v) => writeStored({ v, d: defaultId ?? "" }),
    query,
    personas,
    defaultLook: personaLook(defaultPersona),
    defaultId,
    error: q.error,
    loading: q.isPending,
    retry: () => void q.refetch(),
  };
}

/** Opens the persona manager (a right-hand panel, like Memory, driven by ?personas=). */
export function useOpenPersonaManager() {
  const router = useRouter();
  const pathname = usePathname();
  const search = useSearchParams();
  return () => {
    const q = new URLSearchParams(search.toString());
    q.set("personas", "1");
    router.replace(`${pathname}?${q.toString()}`, { scroll: false });
  };
}

const item =
  "flex h-9 cursor-default select-none items-center gap-2 rounded-md px-2 outline-none data-[highlighted]:bg-hover data-[disabled]:opacity-50";

function Check({ on }: { on: boolean }) {
  return <span className="w-4 flex-none text-accent">{on && <Icon name="check" size={14} strokeWidth={1.8} />}</span>;
}

/** Two overlapping avatars for "All personas". */
function AllAvatars({ personas }: { personas: PersonaLook[] }) {
  const shown = personas.length ? personas.slice(0, 2) : [BUILTIN];
  return (
    <span aria-hidden className="flex flex-none items-center">
      {shown.map((p, i) => (
        <PersonaAvatar key={p.id ?? "builtin"} persona={p} size={16} className={i ? "-ml-1.5 ring-2 ring-surface-raised" : undefined} />
      ))}
    </span>
  );
}

/** The sidebar's top dropdown (mockup frames A and B): default persona, the others, and "All personas". */
export function PersonaFilter({ filter }: { filter: PersonaFilterState }) {
  const manage = useOpenPersonaManager();
  const { personas, defaultLook, defaultId, value } = filter;
  const others = personas.filter((p) => p.id !== defaultId);
  const current = value === "all" ? undefined : value === "default" ? defaultLook : personaLook(others.find((p) => p.id === value));
  const count = (p?: Persona) => (p ? <span className="ml-auto font-mono text-[11px] text-fg-tertiary">{p.chat_count}</span> : null);
  return (
    <div className="flex flex-col gap-1">
      <DropdownMenu.Root>
        <DropdownMenu.Trigger asChild>
          <button
            type="button"
            disabled={filter.loading}
            aria-label={`Show conversations for: ${current ? current.name : "All personas"}${value === "default" ? " (default)" : ""}`}
            className="flex h-[34px] w-full items-center gap-2 rounded-md border border-border bg-surface-raised px-2.5 text-[13px] hover:bg-hover disabled:opacity-60 data-[state=open]:border-accent"
          >
            {current ? (
              <>
                <PersonaAvatar persona={current} />
                <PersonaName persona={current} className="truncate" />
                {value === "default" && <span className="text-xs text-fg-tertiary">(default)</span>}
              </>
            ) : (
              <>
                <AllAvatars personas={personas} />
                <span className="truncate text-fg">All personas</span>
              </>
            )}
            <Icon name="chev" size={13} strokeWidth={1.6} className="ml-auto flex-none text-fg-tertiary" />
          </button>
        </DropdownMenu.Trigger>
        <DropdownMenu.Portal>
          <DropdownMenu.Content align="start" sideOffset={4} className={cx(floatPanel, "w-[280px]")}>
            <DropdownMenu.Label className="px-2 py-1 text-[11px] font-medium uppercase tracking-wide text-fg-tertiary">
              Show conversations for
            </DropdownMenu.Label>
            <DropdownMenu.RadioGroup value={value} onValueChange={filter.set}>
              <DropdownMenu.RadioItem value="default" className={item}>
                <PersonaAvatar persona={defaultLook} />
                <PersonaName persona={defaultLook} className="truncate" />
                <span className="text-xs text-fg-tertiary">(default)</span>
                {count(personas.find((p) => p.id === defaultId))}
                <Check on={value === "default"} />
              </DropdownMenu.RadioItem>
              {others.map((p) => (
                <DropdownMenu.RadioItem key={p.id} value={p.id} className={item}>
                  <PersonaAvatar persona={p} />
                  <PersonaName persona={p} className="truncate" />
                  {count(p)}
                  <Check on={value === p.id} />
                </DropdownMenu.RadioItem>
              ))}
              <DropdownMenu.Separator className="my-1 h-px bg-border-subtle" />
              <DropdownMenu.RadioItem value="all" className={item}>
                <AllAvatars personas={personas} />
                <span className="text-fg">All personas</span>
                <span className="ml-auto" />
                <Check on={value === "all"} />
              </DropdownMenu.RadioItem>
            </DropdownMenu.RadioGroup>
            <DropdownMenu.Separator className="my-1 h-px bg-border-subtle" />
            <DropdownMenu.Item className={item} onSelect={manage}>
              <Icon name="gear" size={14} strokeWidth={1.6} className="text-fg-tertiary" />
              <span className="text-fg-secondary">Manage personas…</span>
            </DropdownMenu.Item>
          </DropdownMenu.Content>
        </DropdownMenu.Portal>
      </DropdownMenu.Root>
      {!!filter.error && (
        <p role="alert" className="flex flex-wrap items-center gap-x-2 px-1 text-xs">
          <span className="text-pill-blocked-fg">Couldn’t load personas.</span>
          <button type="button" className="text-accent hover:underline" onClick={filter.retry}>
            Retry
          </button>
          <span className="basis-full text-fg-tertiary">Showing all conversations.</span>
        </p>
      )}
    </div>
  );
}

/**
 * The persona for a conversation that has no messages yet (mockup frame C): a
 * pill in the composer that opens upwards. Hidden when there's nothing to pick.
 */
export function PersonaSwitcher({
  value,
  onChange,
  disabled,
}: {
  /** The persona id in use, or undefined for the built-in Assistant. */
  value?: string;
  onChange: (id: string) => void;
  disabled?: boolean;
}) {
  const manage = useOpenPersonaManager();
  const q = usePersonas();
  const personas = q.data?.personas ?? [];
  if (!personas.length) return null;
  const defaultId = q.data?.default_persona_id;
  const current = personaLook(personas.find((p) => p.id === value));
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button
          type="button"
          disabled={disabled}
          aria-label={`Persona for this conversation: ${current.name}`}
          className="flex h-[30px] max-w-full items-center gap-1.5 rounded-full border border-accent bg-surface-raised px-2.5 text-xs hover:bg-hover disabled:cursor-not-allowed disabled:opacity-50"
        >
          <PersonaAvatar persona={current} size={18} />
          <PersonaName persona={current} className="truncate" />
          <Icon name="chev" size={12} strokeWidth={1.6} className="rotate-180 text-fg-tertiary" />
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align="start" side="top" sideOffset={6} className={cx(floatPanel, "w-[290px]")}>
          <DropdownMenu.Label className="px-2 py-1 text-[11px] font-medium uppercase tracking-wide text-fg-tertiary">
            Persona for this conversation
          </DropdownMenu.Label>
          <DropdownMenu.RadioGroup value={value ?? ""} onValueChange={onChange}>
            {personas.map((p) => (
              <DropdownMenu.RadioItem key={p.id} value={p.id} className={item}>
                <PersonaAvatar persona={p} />
                <PersonaName persona={p} className="truncate" />
                <span className="truncate text-[11px] text-fg-tertiary">
                  {p.id === defaultId ? "(default) · " : ""}
                  {toneLabel(p.tone)}
                </span>
                <span className="ml-auto" />
                <Check on={p.id === value} />
              </DropdownMenu.RadioItem>
            ))}
          </DropdownMenu.RadioGroup>
          <DropdownMenu.Separator className="my-1 h-px bg-border-subtle" />
          <DropdownMenu.Item className={item} onSelect={manage}>
            <span className="text-xs text-fg-secondary">Manage personas…</span>
          </DropdownMenu.Item>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}

/** "Hi Liam, I'm Atlas." over a large avatar, for a conversation before its first message. */
export function PersonaGreeting({
  persona,
  tone,
  isDefault,
  firstName,
  canSwitch,
}: {
  persona: PersonaLook;
  tone?: Schemas["PersonaTone"];
  isDefault: boolean;
  firstName?: string;
  canSwitch: boolean;
}) {
  return (
    <div className="flex flex-col items-center gap-2 py-10 text-center">
      <PersonaAvatar persona={persona} size={56} className="text-2xl" />
      <h2 className="mt-2 text-xl font-semibold text-fg">
        {firstName ? `Hi ${firstName}, I’m ` : "I’m "}
        <span style={{ color: persona.color }}>{persona.name}</span>.
      </h2>
      <p className="text-[13px] text-fg-secondary">
        A new conversation{isDefault ? " with your default persona" : ""}
        {persona.id ? ` · ${toneLabel(tone)} tone` : ""}
      </p>
      <p className="max-w-[520px] text-[13px] text-fg-secondary">
        {canSwitch
          ? "Pick another persona below before you send your first message."
          : "Ask about your runs, tickets, epics and wiki, or have me draft tickets. I save useful things to your private memory and tell you when I do."}
      </p>
    </div>
  );
}
