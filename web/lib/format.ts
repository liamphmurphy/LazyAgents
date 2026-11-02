// Display formatting. All data renders client-side (TanStack Query), so
// locale and time zone are the viewer's and there's no hydration mismatch.

const MIN = 60_000;
const HOUR = 60 * MIN;
const DAY = 24 * HOUR;

const monthDay = new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric" });
const weekday = new Intl.DateTimeFormat(undefined, { weekday: "short" });
const timeOfDay = new Intl.DateTimeFormat(undefined, { hour: "2-digit", minute: "2-digit", hour12: false });
const fullDate = new Intl.DateTimeFormat(undefined, { weekday: "short", month: "short", day: "numeric" });
const absolute = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" });

/** List-row style: "12m", "5h", "Fri", "Oct 30". */
export function shortAgo(iso: string, now = Date.now()): string {
  const t = new Date(iso).getTime();
  const d = Math.max(0, now - t);
  if (d < MIN) return "now";
  if (d < HOUR) return `${Math.floor(d / MIN)}m`;
  if (d < DAY) return `${Math.floor(d / HOUR)}h`;
  if (d < 6 * DAY) return weekday.format(t);
  return monthDay.format(t);
}

/** Timeline style: "12m ago", "10:48" (today), "Oct 31, 09:12", "Oct 30". */
export function timelineTime(iso: string, now = Date.now()): string {
  const t = new Date(iso);
  const d = now - t.getTime();
  if (d < HOUR) return d < MIN ? "just now" : `${Math.floor(d / MIN)}m ago`;
  if (new Date(now).toDateString() === t.toDateString()) return timeOfDay.format(t);
  if (d < 30 * DAY) return `${monthDay.format(t)}, ${timeOfDay.format(t)}`;
  return monthDay.format(t);
}

export function absoluteTime(iso: string): string {
  return absolute.format(new Date(iso));
}

/** "2026-11-04" (a date, no time zone) → "Wed, Nov 4". */
export function dueDate(ymd: string): string {
  const [y, m, d] = ymd.split("-").map(Number);
  return fullDate.format(new Date(y, m - 1, d));
}
