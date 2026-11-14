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

/** The context line the persona reads, naming what to look up. */
export function workContextLine(s: WorkSubject): string {
  if (s.kind === "ticket") return `Viewing ticket ${s.key} (“${s.title}”).`;
  return `Viewing run ${s.id} (“${s.title}”) by ${s.agent}${s.ticketKey ? ` for ticket ${s.ticketKey}` : ""}.`;
}

/** The body with the subject appended after the marker, once. */
export function withWorkContext(body: string, s: WorkSubject): string {
  const suffix = `${WORK_CONTEXT_MARKER}${workContextLine(s)}`;
  return body.endsWith(suffix) ? body : `${body}${suffix}`;
}

/** The composer chip's label: what the next message carries. */
export function workSubjectLabel(s: WorkSubject): string {
  return s.kind === "ticket" ? `${s.key} · ${s.title}` : `${s.ticketKey ? `${s.ticketKey} · ` : ""}${s.title}`;
}
