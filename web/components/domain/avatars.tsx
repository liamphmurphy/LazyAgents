import { cx } from "@/lib/cx";

/** Agent identity hues, assigned in order (DESIGN.md §2 Agent identity). */
export const AGENT_HUES = ["atlas", "quill", "sentry", "juno", "nova", "relay", "pike", "echo"] as const;
export type AgentHue = (typeof AGENT_HUES)[number];

export function agentHue(colorIndex: number): AgentHue {
  return AGENT_HUES[((colorIndex % AGENT_HUES.length) + AGENT_HUES.length) % AGENT_HUES.length];
}

const AGENT_SIZES = {
  sm: { px: 20, radius: 6, font: 10 }, // inline / table
  md: { px: 24, radius: 7, font: 11 }, // lists
  lg: { px: 38, radius: 10, font: 15 }, // card headers
  stack: { px: 22, radius: 6, font: 10 },
} as const;
export type AgentAvatarSize = keyof typeof AGENT_SIZES;

/** Agents are rounded squares in their identity hue. */
export function AgentAvatar({
  name,
  colorIndex,
  size = "sm",
  ring,
  label = true,
}: {
  name: string;
  colorIndex: number;
  size?: AgentAvatarSize;
  /** Surface color for the stacked ring. */
  ring?: string;
  label?: boolean;
}) {
  const s = AGENT_SIZES[size];
  const hue = agentHue(colorIndex);
  return (
    <span
      role={label ? "img" : undefined}
      aria-label={label ? `${name} (agent)` : undefined}
      aria-hidden={label ? undefined : true}
      className="grid flex-none place-items-center font-semibold"
      style={{
        width: s.px,
        height: s.px,
        borderRadius: s.radius,
        fontSize: s.font,
        color: `var(--agent-${hue})`,
        background: `var(--agent-${hue}-bg)`,
        boxShadow: ring ? `0 0 0 2px ${ring}` : undefined,
      }}
    >
      {name.slice(0, 1).toUpperCase()}
    </span>
  );
}

export function initialsOf(name: string): string {
  const parts = name.trim().split(/\s+/);
  return ((parts[0]?.[0] ?? "") + (parts.length > 1 ? parts[parts.length - 1][0] : "")).toUpperCase();
}

/** People are neutral circles with initials. */
export function HumanAvatar({
  name,
  initials,
  size = 20,
  color,
  ring,
  label = true,
}: {
  name: string;
  initials?: string;
  size?: number;
  /** Optional per-person tint; defaults to the neutral avatar token. */
  color?: string;
  ring?: string;
  label?: boolean;
}) {
  return (
    <span
      role={label ? "img" : undefined}
      aria-label={label ? name : undefined}
      aria-hidden={label ? undefined : true}
      className="grid flex-none place-items-center rounded-full font-semibold text-fg"
      style={{
        width: size,
        height: size,
        fontSize: Math.round(size * 0.42),
        // A personal tint is blended into the theme's avatar color, so the
        // initials stay readable in both light and dark.
        background: color ? `color-mix(in srgb, ${color} 30%, var(--human-avatar-bg))` : "var(--human-avatar-bg)",
        boxShadow: ring ? `0 0 0 2px ${ring}` : undefined,
      }}
    >
      {initials ?? initialsOf(name)}
    </span>
  );
}

export function UnassignedAvatar({ size = 20 }: { size?: number }) {
  return (
    <span
      role="img"
      aria-label="Unassigned"
      className="inline-block flex-none rounded-sm border-[1.5px] border-dashed border-unassigned"
      style={{ width: size, height: size }}
    />
  );
}

export type Actor =
  | { kind: "agent"; name: string; colorIndex: number }
  | { kind: "human"; name: string; initials?: string; color?: string };

export function ActorAvatar({ actor, size = "sm", ring }: { actor: Actor; size?: AgentAvatarSize; ring?: string }) {
  if (actor.kind === "agent") return <AgentAvatar name={actor.name} colorIndex={actor.colorIndex} size={size} ring={ring} />;
  const px = { sm: 20, md: 24, lg: 38, stack: 22 }[size];
  return <HumanAvatar name={actor.name} initials={actor.initials} color={actor.color} size={px} ring={ring} />;
}

/** Overlapping avatars (−5px) with a ring in the surface color, plus "+N". */
export function AvatarStack({
  actors,
  max = 4,
  surface = "var(--surface)",
  className,
}: {
  actors: Actor[];
  max?: number;
  surface?: string;
  className?: string;
}) {
  const shown = actors.slice(0, max);
  const extra = actors.length - shown.length;
  return (
    <span className={cx("flex items-center", className)}>
      {shown.map((a, i) => (
        <span key={`${a.kind}-${a.name}`} className={i ? "-ml-[5px]" : undefined}>
          <ActorAvatar actor={a} size="stack" ring={surface} />
        </span>
      ))}
      {extra > 0 && <span className="ml-1.5 font-mono text-[11px] text-fg-tertiary">+{extra}</span>}
    </span>
  );
}
