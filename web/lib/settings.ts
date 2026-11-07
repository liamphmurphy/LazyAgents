/**
 * Settings rules (LAZYAGENTS-44): which provider secrets exist, what each one
 * unlocks and what breaks without it. Pure so they're unit-tested; secret
 * values never reach the web app (the API returns names and timestamps only).
 */

export type SecretMeta = {
  id: string;
  name: string;
  project?: { key: string; name: string; color: string };
  grants_scope?: string;
  updated_at: string;
  last_used_at?: string;
};

export type Connection = {
  name: string;
  label: string;
  icon: "key" | "github";
  /** Harnesses or capabilities this secret turns on. */
  unlocks: string[];
  /** What breaks while it's unset. */
  without: string;
  /** Runs need it (harness keys). Optional ones don't raise a warning. */
  required: boolean;
};

export const CONNECTIONS: Connection[] = [
  {
    name: "harness:anthropic",
    label: "Anthropic",
    icon: "key",
    unlocks: ["Claude Code", "OpenCode anthropic/…"],
    without: "Claude Code runs fail at start and OpenCode can’t use anthropic/… models.",
    required: true,
  },
  {
    name: "harness:openai",
    label: "OpenAI",
    icon: "key",
    unlocks: ["Codex", "OpenCode openai/…"],
    without: "Codex runs fail at start and OpenCode can’t use openai/… models.",
    required: true,
  },
  {
    name: "git:ssh",
    label: "GitHub SSH key",
    icon: "key",
    unlocks: ["Clone & push over SSH"],
    without: "Runs can’t clone or push project repositories.",
    required: false,
  },
  {
    name: "git:pat",
    label: "GitHub token",
    icon: "github",
    unlocks: ["gh pull requests"],
    without: "Optional with SSH. Without it agents push but can’t open PRs.",
    required: false,
  },
];

const CONNECTION_NAMES = new Set(CONNECTIONS.map((c) => c.name));

export type ConnectionState =
  | { state: "set"; secret: SecretMeta }
  | { state: "projects"; projects: string[] }
  | { state: "unset" };

/** A workspace-wide secret makes a connection "set"; project-only ones cover just those projects. */
export function connectionState(secrets: SecretMeta[], name: string): ConnectionState {
  const own = secrets.filter((s) => s.name === name);
  const ws = own.find((s) => !s.project);
  if (ws) return { state: "set", secret: ws };
  if (own.length) return { state: "projects", projects: own.map((s) => s.project!.name) };
  return { state: "unset" };
}

/** Required connections without a workspace-wide value, in catalog order. */
export function missingConnections(secrets: SecretMeta[]): Connection[] {
  return CONNECTIONS.filter((c) => c.required && connectionState(secrets, c.name).state !== "set");
}

/** Secrets that aren't one of the provider connections (db, deploy, project overrides of other names). */
export function otherSecrets(secrets: SecretMeta[]): SecretMeta[] {
  return secrets.filter((s) => !CONNECTION_NAMES.has(s.name) || s.project);
}

/** Toast after saving: names the harnesses that just became available. */
export function savedMessage(name: string): string {
  const c = CONNECTIONS.find((x) => x.name === name);
  if (!c) return `${name} saved.`;
  return `${name} saved. ${c.unlocks.join(" and ")} ${c.unlocks.length > 1 ? "are" : "is"} now available.`;
}

/** Sidebar dot: secrets can't be stored, or a harness key is missing. */
export function needsAttention(list: { key_configured: boolean; secrets: SecretMeta[] } | undefined): boolean {
  return !!list && (!list.key_configured || missingConnections(list.secrets).length > 0);
}
