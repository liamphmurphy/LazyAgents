// Pure helpers for the persona-led welcome. No React or fetch here so node --test can cover them.

type CarouselPersona = { id: string; chat_count: number };

/** The carousel's order: the default first, then by how often the person chats with each. Ties keep the server's order. */
export function carouselOrder<P extends CarouselPersona>(personas: readonly P[], defaultId: string | undefined): P[] {
  return personas
    .map((p, i) => ({ p, i }))
    .sort((a, b) => Number(b.p.id === defaultId) - Number(a.p.id === defaultId) || b.p.chat_count - a.p.chat_count || a.i - b.i)
    .map((x) => x.p);
}

/** Up to two neighbours on each side of the picked persona, wrapping round, never repeating anyone. */
export function carouselWindow<P>(ordered: readonly P[], index: number, perSide = 2): { left: P[]; right: P[] } {
  const n = ordered.length;
  if (n <= 1 || index < 0) return { left: [], right: [] };
  const others = n - 1;
  const rightCount = Math.min(perSide, Math.ceil(others / 2));
  const leftCount = Math.min(perSide, others - rightCount);
  const at = (k: number) => ordered[(((index + k) % n) + n) % n];
  return {
    left: Array.from({ length: leftCount }, (_, i) => at(i - leftCount)),
    right: Array.from({ length: rightCount }, (_, i) => at(i + 1)),
  };
}

type Capabilities = { code?: boolean; images?: boolean; svg?: boolean; personas?: boolean; workflows?: boolean };
const DOES: [keyof Capabilities, string][] = [
  ["code", "writes code"],
  ["images", "makes images"],
  ["svg", "draws diagrams"],
  ["workflows", "builds workflows"],
  ["personas", "proposes personas"],
];

/** The line under a persona's name: its tone, then what it can do. Personas have no description of their own. */
export function personaTagline(p: { tone: string; capabilities?: Capabilities } | undefined, toneLabel: (t: string) => string, isDefault: boolean): string {
  if (!p) return "Your built-in assistant: runs, tickets, the wiki and the web.";
  const does = DOES.filter(([k]) => p.capabilities?.[k]).map(([, label]) => label);
  return [`${toneLabel(p.tone)} tone`, ...does, ...(isDefault ? ["your default"] : [])].join(" · ");
}

/** Starters when the personalised ones aren't written yet. */
export const FALLBACK_STARTERS = ["What did my agents finish this week?", "Draft tickets from a spec", "Think something through with me"];

export type PickUpTone = "human" | "persona" | "neutral";
export type PickUpCard = { key: string; tone: PickUpTone; label: string; title: string; meta: string; href: string; color?: string };

type InboxLike = { id: string; title: string; href: string; created_at: string; resolved: boolean; read: boolean; agent?: { name: string } };
type DockLike = {
  persona: { name: string; color: string };
  chats: { id: string; title: string; state: "working" | "replied" | "needs_you"; unread: number; last_line: string }[];
};
type ChatLike = { id: string; title: string; message_count: number; archived: boolean; created_at: string; last_message_at?: string; persona?: { name: string; color: string } | null };

/**
 * "Pick up where you left off": what needs the person first (open inbox items, chats
 * waiting on them), then replies they haven't read, then their latest conversations.
 * At most `max`, each place once.
 */
export function pickUpCards(
  inbox: readonly InboxLike[],
  dock: readonly DockLike[],
  chats: readonly ChatLike[],
  ago: (iso: string) => string,
  max = 3,
): PickUpCard[] {
  const out: PickUpCard[] = [];
  const seen = new Set<string>();
  const add = (c: PickUpCard) => {
    if (out.length >= max || seen.has(c.href)) return;
    seen.add(c.href);
    out.push(c);
  };
  for (const i of inbox) {
    if (i.resolved || i.read) continue;
    add({ key: `inbox:${i.id}`, tone: "human", label: "Needs you", title: i.title, meta: [i.agent?.name, ago(i.created_at)].filter(Boolean).join(" · "), href: i.href });
  }
  for (const e of dock) {
    for (const c of e.chats) {
      if (c.state === "needs_you") add({ key: `dock:${c.id}`, tone: "human", label: "Needs you", title: c.title || "New conversation", meta: e.persona.name, href: `/chat/${c.id}` });
    }
  }
  for (const e of dock) {
    for (const c of e.chats) {
      if (c.state === "replied" && c.unread > 0) {
        add({ key: `dock:${c.id}`, tone: "persona", label: "Replied", title: c.last_line || c.title || "New conversation", meta: `${e.persona.name} · ${c.title || "New conversation"}`, href: `/chat/${c.id}`, color: e.persona.color });
      }
    }
  }
  const recent = chats
    .filter((c) => !c.archived && c.message_count > 0)
    .toSorted((a, b) => Date.parse(b.last_message_at ?? b.created_at) - Date.parse(a.last_message_at ?? a.created_at));
  for (const c of recent) {
    add({ key: `chat:${c.id}`, tone: "neutral", label: "Continue", title: c.title || "New conversation", meta: [c.persona?.name ?? "Assistant", ago(c.last_message_at ?? c.created_at)].join(" · "), href: `/chat/${c.id}` });
  }
  return out;
}

type CatalogGroup<H extends string> = { harness: H; available: boolean; models: readonly { id: string; default: boolean }[] };
type CatalogAgent = { kind: string; model: string };

/**
 * The harness and model a chat started from the welcome gets when the person doesn't pick one,
 * for the model picker to show. The server starts every chat on the Personal Assistant's model;
 * before its first chat there is none yet, and it takes a main agent's (the first listed, an
 * approximation of the oldest). The harness is the catalog's group listing that model, else the
 * first available harness's default.
 */
export function newChatSelection<H extends string>(groups: readonly CatalogGroup<H>[] | undefined, agents: readonly CatalogAgent[]): { harness: H; model: string } | undefined {
  if (!groups?.length) return undefined;
  const model = (agents.find((a) => a.kind === "assistant") ?? agents.find((a) => a.kind === "main"))?.model;
  const owner = model ? groups.find((g) => g.models.some((m) => m.id === model)) : undefined;
  if (owner && model) return { harness: owner.harness, model };
  const g = groups.find((x) => x.available) ?? groups[0];
  const m = g.models.find((x) => x.default) ?? g.models[0];
  return m ? { harness: g.harness, model: m.id } : undefined;
}
