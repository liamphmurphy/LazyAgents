// A chat's playground in the web app: its card's fence, where the browser
// reaches it, and the logs its gateway streams. Kept free of React so
// `npm run test:unit` covers it.

export const isPlaygroundFence = (info: string | undefined) => (info ?? "").trim().split(/\s+/)[0]?.toLowerCase() === "playground";

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/** The playground id in a fence, or null when it holds anything else. */
export function parsePlaygroundFence(code: string): string | null {
  const id = code.trim();
  return UUID.test(id) ? id.toLowerCase() : null;
}

/**
 * Where the browser reaches a playground: Vesper's own host, on the runner's
 * port. Same host is what lets the runner recognise the Vesper session, so it
 * follows however the person reached Vesper (localhost, 127.0.0.1, a LAN name).
 */
export function playgroundOrigin(loc: { protocol: string; hostname: string }, port: number): string {
  const host = loc.hostname.includes(":") && !loc.hostname.startsWith("[") ? `[${loc.hostname}]` : loc.hostname;
  return `${loc.protocol}//${host}:${port}`;
}

/** A path the person typed, as one to load: always absolute, never another origin. */
export function normalizePath(input: string): string {
  let p = input.trim();
  const scheme = /^[a-z][a-z0-9+.-]*:\/\/[^/]*/i.exec(p);
  if (scheme) p = p.slice(scheme[0].length);
  if (!p.startsWith("/")) p = "/" + p;
  return p.replace(/^\/{2,}/, "/");
}

export type LogLine = { t: string; source: string; stream: "stdout" | "stderr"; text: string };

/** Logs kept in the browser; older lines drop off. */
export const MAX_LOG_LINES = 2000;

export function appendLogs(lines: LogLine[], more: LogLine[], max = MAX_LOG_LINES): LogLine[] {
  const out = lines.concat(more);
  return out.length > max ? out.slice(out.length - max) : out;
}

const ERROR = /\b(error|panic|exception|fatal|traceback)\b/i;

/** Whether a line reports a failure, for the Logs tab's count and colour. */
export const isErrorLine = (l: LogLine) => ERROR.test(l.text);

/** The sources lines came from, in first-seen order. */
export function logSources(lines: LogLine[]): string[] {
  const seen: string[] = [];
  for (const l of lines) if (!seen.includes(l.source)) seen.push(l.source);
  return seen;
}

/**
 * Lines written while a request was in flight, with some slack after it: a
 * request carries no id the app's logs would print, so time is the only link.
 */
export function linesDuring(lines: LogLine[], sentAt: number, doneAt: number, slackMs = 1500): LogLine[] {
  return lines.filter((l) => {
    const t = Date.parse(l.t);
    return t >= sentAt - 250 && t <= doneAt + slackMs;
  });
}

/** "6 min ago", "just now", "2 h ago". */
export function ago(iso: string | null | undefined, now: number): string {
  if (!iso) return "";
  const s = Math.max(0, (now - Date.parse(iso)) / 1000);
  if (s < 60) return "just now";
  if (s < 3600) return `${Math.floor(s / 60)} min ago`;
  return `${Math.floor(s / 3600)} h ago`;
}

/** The footer's idle rule, e.g. "stops after 30 min idle". */
export function idleRule(seconds: number | null | undefined): string {
  if (!seconds) return "stops when unused";
  const min = Math.round(seconds / 60);
  return min >= 60 && min % 60 === 0 ? `stops after ${min / 60} h idle` : `stops after ${Math.max(1, min)} min idle`;
}

/** Which document the API tab got, from the gateway's X-Vesper-Spec-Source. */
export type SpecSource =
  | { kind: "served"; path: string } // the running app serves it
  | { kind: "file"; path: string } // declared in the workspace
  | { kind: "inferred"; path: string; commit: string } // the persona wrote it from the code
  | { kind: "unknown" };

export function parseSpecSource(header: string | null): SpecSource {
  const i = header?.indexOf(":") ?? -1;
  if (!header || i < 0) return { kind: "unknown" };
  const kind = header.slice(0, i);
  const rest = header.slice(i + 1);
  if (!rest) return { kind: "unknown" };
  if (kind === "served" || kind === "file") return { kind, path: rest };
  if (kind === "inferred") {
    const at = rest.lastIndexOf("@");
    return at > 0 ? { kind, path: rest.slice(0, at), commit: rest.slice(at + 1) } : { kind, path: rest, commit: "" };
  }
  return { kind: "unknown" };
}

/** Whether the checkout moved past the commit an inferred document was read from (hashes may be short). */
export function inferredIsStale(readFrom: string, head: string): boolean {
  if (!readFrom || !head) return false;
  return !(head.startsWith(readFrom) || readFrom.startsWith(head));
}

/**
 * A request's log lines: those naming its id (the gateway's X-Request-Id) when
 * the app logs it, which is exact; else those written while it ran.
 */
export function linesForRequest(lines: LogLine[], id: string, sentAt: number, doneAt: number): { by: "id" | "time"; lines: LogLine[] } {
  const byId = id ? lines.filter((l) => l.text.includes(id)) : [];
  return byId.length ? { by: "id", lines: byId } : { by: "time", lines: linesDuring(lines, sentAt, doneAt) };
}

/** What the API tab asks the persona when there's no document, or the inferred one is out of date. */
export function mapApiPrompt({ stale, existing }: { stale: boolean; existing: string | null }): string {
  if (existing) {
    return `The code changed since you mapped the playground's API${stale ? "" : ", or I want it checked again"}. Read the routes and handlers again and update ${existing}: every route, with an x-source of file:line on each operation, then call set_playground_openapi with it, inferred, and the commit you read.`;
  }
  return "The playground's app has no OpenAPI document. Read its routes and handlers and infer an OpenAPI document for the API tab: every route, with an x-source of file:line on each operation, then call set_playground_openapi with it, inferred, and the commit you read.";
}
