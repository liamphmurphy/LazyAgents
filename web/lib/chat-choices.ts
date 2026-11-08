// Decision cards in chat: a ```choices fence holds a question and two to six options
// the person answers with a click (components/domain/choices-fence). Kept free of
// React so `npm run test:unit` covers it.

export const isChoicesFence = (info: string | undefined) => (info ?? "").trim().split(/\s+/)[0]?.toLowerCase() === "choices";

export type Choice = { label: string; detail: string };
export type Choices = { question: string; options: Choice[] };

const OPTION = /^\s*[-*+]\s+(.+)$/;
const MAX_OPTIONS = 6;

/** The fence's question and options, or null unless it offers at least two. */
export function parseChoices(code: string): Choices | null {
  const question: string[] = [];
  const options: Choice[] = [];
  for (const line of code.split("\n")) {
    const m = OPTION.exec(line);
    if (m) {
      const [label, ...detail] = m[1].split("|");
      if (label.trim()) options.push({ label: label.trim(), detail: detail.join("|").trim() });
    } else if (line.trim() && options.length === 0) question.push(line.trim());
  }
  return options.length >= 2 && options.length <= MAX_OPTIONS ? { question: question.join(" "), options } : null;
}

/** The message a click sends. A label can't mention a persona or carry formatting. */
export function choicePrompt(label: string): string {
  return `Going with: ${label.replace(/[[\]`@\n]/g, "").trim()}`;
}

/** Which option a later message picked, if it was one of these. */
export function chosenOption(options: Choice[], next: string | undefined): string | undefined {
  return next ? options.find((o) => next.endsWith(choicePrompt(o.label)))?.label : undefined;
}
