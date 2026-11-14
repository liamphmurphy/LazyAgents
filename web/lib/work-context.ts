/**
 * The work chat beside a run or ticket (components/domain/work-chat.tsx): what it tells the persona
 * the person is looking at. Pure so the wording and marker are unit-tested; the server cuts a
 * message's request text at the same marker (server/internal/runs/chat_personas.go), so a run's
 * title can't mention a persona or set a hand-off limit.
 */

export const WORK_CONTEXT_MARKER = "\n\n<!-- vesper:work-context -->\n";

/** What the main pane shows beside the work chat. */
export type WorkSubject =
  | { kind: "run"; id: string; title: string; agent: string; ticketKey?: string }
  | { kind: "ticket"; key: string; title: string };

/** The context line the persona reads, naming what to look up; a pinned item says it was kept on purpose. */
export function workContextLine(s: WorkSubject, pinned = false): string {
  const verb = pinned ? "Pinned" : "Viewing";
  if (s.kind === "ticket") return `${verb} ticket ${s.key} (“${s.title}”).`;
  return `${verb} run ${s.id} (“${s.title}”) by ${s.agent}${s.ticketKey ? ` for ticket ${s.ticketKey}` : ""}.`;
}

/** Whether two subjects are the same run or ticket. */
export function sameSubject(a: WorkSubject, b: WorkSubject): boolean {
  return a.kind === "ticket" ? b.kind === "ticket" && a.key === b.key : b.kind === "run" && a.id === b.id;
}

/**
 * The body with the context appended after the marker, once: the item in view, and the pinned
 * one first when it is a different item, so "this" can mean either and the persona knows which
 * the person chose to keep. On a page with no run or ticket in view (the companion beside Library,
 * Settings…) only the pin goes, and with neither the body goes as it is.
 */
export function withWorkContext(body: string, viewing: WorkSubject | undefined, pinned?: WorkSubject): string {
  const same = !!pinned && !!viewing && sameSubject(pinned, viewing);
  const lines = [pinned && !same ? workContextLine(pinned, true) : undefined, viewing ? workContextLine(viewing, same) : undefined].filter(Boolean);
  if (!lines.length) return body;
  const suffix = `${WORK_CONTEXT_MARKER}${lines.join("\n")}`;
  return body.endsWith(suffix) ? body : `${body}${suffix}`;
}

/** Reads a stored pin, dropping anything that isn't a run or ticket subject. */
export function parsePin(raw: string | null): WorkSubject | undefined {
  if (!raw) return undefined;
  try {
    const v = JSON.parse(raw) as Partial<WorkSubject>;
    if (v?.kind === "ticket" && typeof v.key === "string" && typeof v.title === "string") return { kind: "ticket", key: v.key, title: v.title };
    if (v?.kind === "run" && typeof v.id === "string" && typeof v.title === "string" && typeof v.agent === "string")
      return { kind: "run", id: v.id, title: v.title, agent: v.agent, ...(typeof v.ticketKey === "string" ? { ticketKey: v.ticketKey } : {}) };
  } catch {
    /* A corrupt pin is no pin. */
  }
  return undefined;
}

/** The composer chip's label: what the next message carries. */
export function workSubjectLabel(s: WorkSubject): string {
  return s.kind === "ticket" ? `${s.key} · ${s.title}` : `${s.ticketKey ? `${s.ticketKey} · ` : ""}${s.title}`;
}
