/** How scheduled tasks read in the UI. Pure so the wording is unit-tested. */

type Schedule = { frequency: string; weekdays: number[]; month_day: number; at_time: string; cron: string };
type RunLike = { state: string };
type TaskLike = { enabled: boolean; runs: RunLike[] };

const DAYS = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];

export function clockLabel(at: string): string {
  const [h, m] = at.split(":").map(Number);
  return `${h % 12 || 12}:${String(m).padStart(2, "0")} ${h < 12 ? "AM" : "PM"}`;
}

function ordinal(n: number): string {
  const tail = n % 100 >= 11 && n % 100 <= 13 ? 0 : n % 10;
  return `${n}${["th", "st", "nd", "rd"][tail] ?? "th"}`;
}

export function describeSchedule(s: Schedule): string {
  const at = clockLabel(s.at_time);
  switch (s.frequency) {
    case "daily":
      return `Every day at ${at}`;
    case "weekly": {
      const days = [...s.weekdays].sort();
      if (days.join() === "1,2,3,4,5") return `Weekdays at ${at}`;
      if (days.length === 7) return `Every day at ${at}`;
      return `Every ${days.map((d) => DAYS[d]).join(", ")} at ${at}`;
    }
    case "monthly":
      return `${ordinal(s.month_day)} of the month at ${at}`;
    default:
      return `Cron: ${s.cron}`;
  }
}

export const RUN_LABEL: Record<string, { label: string; tone: "done" | "human" | "blocked" | "neutral" }> = {
  running: { label: "Running", tone: "neutral" },
  waiting: { label: "Needs you", tone: "human" },
  done: { label: "Done", tone: "done" },
  failed: { label: "Failed", tone: "blocked" },
};

/** The nav badge: tasks with a run waiting on the person. */
export function tasksNeedingYou(tasks: TaskLike[] | undefined): number {
  return (tasks ?? []).filter((t) => t.runs[0]?.state === "waiting").length;
}

export function moneyLabel(micros: number): string {
  return `$${(micros / 1_000_000).toFixed(2)}`;
}
