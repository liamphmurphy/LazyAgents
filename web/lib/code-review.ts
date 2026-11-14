// The Code dock's pure parts (components: app/(app)/chat/_components/code-panel and push-requests).
// Kept free of React so `npm run test:unit` covers it.

import type { Schemas } from "@/lib/api/client";

type Change = Schemas["ChatCodeChange"];

/** A comment the person left on one line of the diff, sent to the persona with the others as one review. */
export type LineComment = {
  path: string;
  /** The line's number in the new file, or in the old one for a removed line. */
  line: number;
  side: "new" | "old";
  /** The line itself, quoted so the persona sees what was meant even if the file moved on. */
  code: string;
  body: string;
};

/** A comment's identity: one per line and side. */
export const commentKey = (c: Pick<LineComment, "path" | "line" | "side">) => `${c.side}:${c.path}:${c.line}`;

/** The repository's short name: owner/name from a GitHub URL, else the URL. */
export function repoName(url: string): string {
  return url.replace(/^https:\/\/github\.com\//, "").replace(/^git@github\.com:/, "").replace(/\.git$/, "");
}

/**
 * The review as the message the persona gets: each comment anchored to file:line with the line
 * quoted, in file and line order, like a GitHub review. Empty comments are left out.
 */
export function reviewMessage(comments: readonly LineComment[], branch?: string): string {
  const kept = comments.filter((c) => c.body.trim());
  if (!kept.length) return "";
  const sorted = [...kept].sort((a, b) => a.path.localeCompare(b.path) || a.line - b.line);
  const parts = sorted.map((c) => {
    const where = `\`${c.path}:${c.line}\`${c.side === "old" ? " (removed line)" : ""}`;
    const quote = c.code.trim() ? `\n  > ${c.code.trim().slice(0, 200)}` : "";
    return `- ${where}${quote}\n  ${c.body.trim().replace(/\n/g, "\n  ")}`;
  });
  const n = kept.length;
  return `Review of ${branch ? `\`${branch}\`` : "your changes"} (${n} ${n === 1 ? "comment" : "comments"}). Please address each:\n\n${parts.join("\n")}`;
}

/** Files grouped by directory for the dock's tree: directories in order, files sorted within. */
export function fileTree(files: readonly Change[]): { dir: string; files: Change[] }[] {
  const byDir = new Map<string, Change[]>();
  for (const f of files) {
    const i = f.path.lastIndexOf("/");
    const dir = i < 0 ? "" : f.path.slice(0, i);
    byDir.set(dir, [...(byDir.get(dir) ?? []), f]);
  }
  return [...byDir.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([dir, fs]) => ({ dir, files: fs.sort((a, b) => a.path.localeCompare(b.path)) }));
}

/** The file's name without its directory. */
export const baseName = (path: string) => path.slice(path.lastIndexOf("/") + 1);

/** What a push card says while it isn't waiting on the person. */
export function pushStateLabel(r: Pick<Schemas["PushRequest"], "state" | "auto" | "pr_number">, persona: string): string {
  switch (r.state) {
    case "staging":
      return `Staged when ${persona}'s turn ends`;
    case "pending":
      return "Waiting for you";
    case "pushing":
      return "Pushing…";
    case "pushed":
      return `${r.auto ? "Pushed automatically" : "Pushed"}${r.pr_number ? ` · PR #${r.pr_number}` : ""}`;
    case "declined":
      return "Declined";
    case "failed":
      return "Not pushed";
  }
}
