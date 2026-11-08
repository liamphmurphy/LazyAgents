// Deep research in chat: a ```research-plan fence holds a title and the questions a
// persona proposes (components/domain/research-fence). Kept free of React so
// `npm run test:unit` covers it.

export const isResearchPlanFence = (info: string | undefined) => (info ?? "").trim().split(/\s+/)[0]?.toLowerCase() === "research-plan";

export type PlanQuestion = { label: string; question: string };
export type ResearchPlan = { title: string; questions: PlanQuestion[] };

const LINE = /^\s*[-*+]\s+(.+)$/;
export const MAX_QUESTIONS = 5;
export const SOURCE_RANGE = { min: 5, max: 200 };
export const MINUTE_RANGE = { min: 5, max: 120 };
export const SPEND_OPTIONS = [3, 6, 12] as const;
export type Spend = number | "unlimited";

/** The fence's title and questions, or null when it has none to research. */
export function parseResearchPlan(code: string): ResearchPlan | null {
  let title = "";
  const questions: PlanQuestion[] = [];
  for (const line of code.split("\n")) {
    const m = LINE.exec(line);
    if (m) {
      const [label, ...rest] = m[1].split("|");
      const question = rest.join("|").trim();
      if (label.trim()) questions.push(question ? { label: label.trim(), question } : { label: label.trim().slice(0, 40), question: label.trim() });
    } else if (line.trim() && !title) title = line.trim();
  }
  return title && questions.length > 0 && questions.length <= MAX_QUESTIONS ? { title, questions } : null;
}

/** Whether a plan can be sent as it stands. */
export function planReady(p: ResearchPlan, sources: number, minutes: number): boolean {
  return (
    !!p.title.trim() &&
    p.questions.length > 0 &&
    p.questions.length <= MAX_QUESTIONS &&
    p.questions.every((q) => q.label.trim() && q.question.trim()) &&
    sources >= SOURCE_RANGE.min && sources <= SOURCE_RANGE.max &&
    minutes >= MINUTE_RANGE.min && minutes <= MINUTE_RANGE.max
  );
}

const ACTIVE = new Set(["running", "finishing", "ready", "writing"]);
export const researchActive = (state: string) => ACTIVE.has(state);

/** "18 min", or "<1 min" for a job that barely started. */
export function minutesLabel(ms: number): string {
  const m = Math.round(ms / 60000);
  return m < 1 ? "<1 min" : `${m} min`;
}

/** The spend half of a progress line; nothing to count against when there's no limit. */
export function spendLabel(spent: number, limit: number | undefined): string {
  return limit === undefined ? "no spend limit" : `$${spent.toFixed(2)} of $${limit % 1 ? limit.toFixed(2) : limit}`;
}

/** Progress toward the end of reading: finished researchers, or pages out of the limit, whichever is further. */
export function researchProgress(tasks: { state: string }[], sourcesRead: number, maxSources: number): number {
  const finished = tasks.length ? tasks.filter((t) => t.state !== "queued" && t.state !== "running").length / tasks.length : 0;
  return Math.min(1, Math.max(finished, sourcesRead / maxSources));
}

const ATX2 = /^##\s+(.+?)\s*#*\s*$/;

/** The report's sections for jumping to them, with line numbers matching headingAnchors. */
export function reportSections(md: string, anchors: Map<number, string>): { title: string; anchor: string }[] {
  const out: { title: string; anchor: string }[] = [];
  md.split("\n").forEach((line, i) => {
    const m = ATX2.exec(line);
    const anchor = anchors.get(i + 1) ?? anchors.get(i);
    if (m && anchor) out.push({ title: m[1], anchor });
  });
  return out;
}
