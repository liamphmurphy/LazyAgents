"use client";

import { useRouter } from "next/navigation";
import { useState, useSyncExternalStore, type CSSProperties, type ReactNode } from "react";
import { DropdownMenu } from "radix-ui";
import type { Schemas } from "@/lib/api/client";
import { usePersonaDock } from "@/lib/api/hooks";
import { usePersonas } from "@/lib/api/persona-hooks";
import { cx } from "@/lib/cx";
import { DOCK_STATE_LABEL, dockEntries, type DockState } from "@/lib/persona-dock";
import { repoShortName, splitPersonaStrip } from "@/lib/persona-ui";
import { Icon, type IconName } from "@/components/ui/icon";
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

/** What a persona can be allowed to do, in display order. */
export const CAPABILITIES: { key: "code" | "images" | "svg" | "personas" | "workflows"; icon: IconName; label: string; title: string }[] = [
  { key: "code", icon: "terminal", label: "Write code", title: "Can write code" },
  { key: "images", icon: "artifacts", label: "Generate images", title: "Can generate images" },
  { key: "svg", icon: "library", label: "Create SVG images", title: "Can create SVG images" },
  { key: "personas", icon: "users", label: "Create personas", title: "Can propose new personas" },
  { key: "workflows", icon: "graph", label: "Create workflows", title: "Can propose workflows" },
];

/** Small icons for what a persona can do (code, images, SVG images, create personas, create workflows); nothing when it can't do any. */
export function CapabilityBadges({ capabilities, className }: { capabilities?: Schemas["PersonaCapabilities"]; className?: string }) {
  const granted = CAPABILITIES.filter((c) => capabilities?.[c.key]);
  if (!granted.length) return null;
  return (
    <span className={cx("inline-flex flex-none items-center gap-1 text-fg-tertiary", className)} data-testid="capability-badges">
      {granted.map((c) => (
        <span key={c.key} title={c.title} className="inline-flex items-center rounded bg-selected p-0.5">
          <Icon name={c.icon} size={12} />
          <span className="sr-only">{c.title}</span>
        </span>
      ))}
    </span>
  );
}

/** Which of the persona's repositories a new conversation works in. */
export function RepoSelect({ repos, value, onChange, disabled }: { repos: string[]; value: string; onChange: (url: string) => void; disabled?: boolean }) {
  return (
    <label className="inline-flex h-[30px] max-w-full items-center gap-1.5 rounded-full border border-border bg-surface-raised px-2.5 text-xs text-fg-secondary">
      <Icon name="branch" size={13} className="flex-none" />
      <select
        aria-label="Repository for this conversation"
        value={value}
        disabled={disabled}
        title={value}
        onChange={(e) => onChange(e.target.value)}
        className="min-w-0 max-w-[220px] cursor-pointer truncate bg-transparent font-mono text-xs text-fg outline-none disabled:cursor-not-allowed disabled:opacity-50"
      >
        {repos.map((r) => (
          <option key={r} value={r}>
            {repoShortName(r)}
          </option>
        ))}
      </select>
    </label>
  );
}

/** Avatar image cropped to a circle, or the initial on a tint of the persona color. */
export function PersonaAvatar({ persona, size = 20, className }: { persona: PersonaLook; size?: number; className?: string }) {
  const style: CSSProperties = { width: size, height: size };
  // An avatar that fails to load (removed, stale ?v=) falls back to the initial.
  const [broken, setBroken] = useState<string>();
  if (persona.avatar_url && broken !== persona.avatar_url) {
    const src = persona.avatar_url;
    return (
      // A same-origin, private, cache-busted URL: next/image would only add a proxy hop.
      // eslint-disable-next-line @next/next/no-img-element
      <img src={src} alt="" aria-hidden onError={() => setBroken(src)} className={cx("flex-none rounded-full object-cover", className)} style={style} />
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

/** Overlapping avatars of everyone in a group chat, host first. */
export function PersonaStack({ personas, size = 16 }: { personas: PersonaLook[]; size?: number }) {
  return (
    <span className="flex flex-none items-center" data-testid="persona-stack">
      {personas.map((p, i) => (
        <span key={p.id ?? p.name} className="rounded-full ring-2 ring-surface" style={{ marginLeft: i ? -Math.round(size / 3) : 0, zIndex: personas.length - i }}>
          <PersonaAvatar persona={p} size={size} />
        </span>
      ))}
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

/** Opens the persona manager (LAZYAGENTS-85). */
export function useOpenPersonaManager() {
  const router = useRouter();
  return () => router.push("/personas");
}

const item =
  "flex h-9 cursor-default select-none items-center gap-2 rounded-md px-2 outline-none data-[highlighted]:bg-hover data-[disabled]:opacity-50";

function Check({ on }: { on: boolean }) {
  return <span className="w-4 flex-none text-accent">{on && <Icon name="check" size={14} strokeWidth={1.8} />}</span>;
}

const ACTIVITY_DOT: Record<DockState, string> = { working: "bg-state-done", replied: "bg-accent", needs_you: "bg-state-human" };

/** Each persona's dock state, for the dots on its avatar in the menu. */
export function usePersonaActivity(viewed: string | null): Map<string, DockState> {
  const dock = usePersonaDock();
  const entries = dockEntries<Schemas["PersonaDockChat"], Schemas["PersonaDockEntry"]>(dock.data?.personas ?? [], viewed);
  return new Map(entries.map((e) => [e.persona.id ?? "", e.state]));
}

/** One round avatar button: ringed when picked, with a dot when the persona has news. */
export function PersonaAvatarButton({ label, on, state, disabled, onClick, children }: { label: string; on: boolean; state?: DockState; disabled?: boolean; onClick: () => void; children: ReactNode }) {
  const name = state ? `${label}: ${DOCK_STATE_LABEL[state]}` : label;
  return (
    <button type="button" aria-label={name} aria-pressed={on} title={name} disabled={disabled} onClick={onClick}
      className={cx("relative grid size-6 flex-none place-items-center rounded-full disabled:opacity-60", on ? "outline-2 outline-accent" : "hover:opacity-80")}>
      {children}
      {state && <span aria-hidden className={cx("absolute -right-0.5 -top-0.5 size-2 rounded-full ring-2 ring-sidebar", ACTIVITY_DOT[state])} />}
    </button>
  );
}

/** The menu's persona picker in Chat mode: All, the default, the most-used others, and the rest behind "+N". */
export function PersonaStrip({ filter, activity }: { filter: PersonaFilterState; activity: Map<string, DockState> }) {
  const manage = useOpenPersonaManager();
  const { personas, defaultLook, defaultId, value } = filter;
  const { shown, overflow } = splitPersonaStrip(personas.filter((p) => p.id !== defaultId), value);
  return (
    <div className="flex flex-col gap-1">
      <div role="group" aria-label="Show conversations for" className="flex items-center gap-0.5 px-0.5 py-0.5">
        <PersonaAvatarButton label="All personas" on={value === "all"} disabled={filter.loading} onClick={() => filter.set("all")}>
          <span className="grid size-[22px] place-items-center rounded-full border border-border-strong bg-surface text-fg-secondary"><Icon name="users" size={12} /></span>
        </PersonaAvatarButton>
        <PersonaAvatarButton label={`${defaultLook.name} (default)`} on={value === "default"} state={defaultId ? activity.get(defaultId) : undefined} disabled={filter.loading} onClick={() => filter.set("default")}>
          <PersonaAvatar persona={defaultLook} size={22} />
        </PersonaAvatarButton>
        {shown.map((p) => (
          <PersonaAvatarButton key={p.id} label={p.name} on={value === p.id} state={activity.get(p.id)} onClick={() => filter.set(p.id)}>
            <PersonaAvatar persona={p} size={22} />
          </PersonaAvatarButton>
        ))}
        {overflow.length > 0 && (
          <DropdownMenu.Root>
            <DropdownMenu.Trigger asChild>
              <button type="button" aria-label="More personas" title="More personas"
                className="grid size-6 flex-none place-items-center rounded-full border border-border-strong bg-surface text-[10.5px] font-semibold text-fg-secondary hover:bg-hover data-[state=open]:border-accent">
                +{overflow.length}
              </button>
            </DropdownMenu.Trigger>
            <DropdownMenu.Portal>
              <DropdownMenu.Content align="end" sideOffset={4} className={cx(floatPanel, "w-[240px]")}>
                {overflow.map((p) => (
                  <DropdownMenu.Item key={p.id} className={item} onSelect={() => filter.set(p.id)}>
                    <PersonaAvatar persona={p} />
                    <PersonaName persona={p} className="truncate" />
                    <span className="ml-auto font-mono text-[11px] text-fg-tertiary">{p.chat_count}</span>
                  </DropdownMenu.Item>
                ))}
                <DropdownMenu.Separator className="my-1 h-px bg-border-subtle" />
                <DropdownMenu.Item className={item} onSelect={manage}>
                  <Icon name="gear" size={14} strokeWidth={1.6} className="text-fg-tertiary" />
                  <span className="text-fg-secondary">Manage personas…</span>
                </DropdownMenu.Item>
              </DropdownMenu.Content>
            </DropdownMenu.Portal>
          </DropdownMenu.Root>
        )}
      </div>
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
                <CapabilityBadges capabilities={p.capabilities} />
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
        {persona.id ? <span style={{ color: persona.color }}>{persona.name}</span> : "your assistant"}.
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
