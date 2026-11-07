/**
 * Inline check for a project's GitHub repository (LAZYAGENTS-5, LAZYAGENTS-44).
 * Mirrors canonicalRepoURL in server/internal/projects/service.go: SSH URLs
 * are kept, HTTPS GitHub links are saved in SSH form, anything else fails.
 * The server stays the authority; this only explains the result while typing.
 */

export type RepoCheck =
  | { state: "empty" }
  | { state: "ok"; kind: "ssh" | "https"; saved: string }
  | { state: "invalid"; message: string };

const SSH = /^(?:git@github\.com:|ssh:\/\/git@github\.com\/)[A-Za-z0-9][A-Za-z0-9-]{0,99}\/[A-Za-z0-9._-]+$/;
const HTTPS = /^https:\/\/(?:www\.)?github\.com\/([A-Za-z0-9][A-Za-z0-9-]{0,99})\/([A-Za-z0-9._-]+?)(?:\.git)?$/;
const BAD_NAME = /\/\.{1,2}(?:\.git)?$/;

export const REPO_URL_HELP = "Use git@github.com:owner/repo.git or https://github.com/owner/repo.";

export function checkRepoURL(input: string): RepoCheck {
  let u = input.trim().replace(/\/+$/, "");
  if (!u) return { state: "empty" };
  let kind: "ssh" | "https" = "ssh";
  const m = HTTPS.exec(u);
  if (m) {
    u = `git@github.com:${m[1]}/${m[2]}.git`;
    kind = "https";
  }
  if (SSH.test(u) && !BAD_NAME.test(u)) return { state: "ok", kind, saved: u };
  if (/^(?:https?:\/\/|git@|ssh:\/\/)/i.test(u) && !/github\.com[:/]/i.test(u)) {
    return { state: "invalid", message: `Only GitHub repositories work. ${REPO_URL_HELP}` };
  }
  if (/^http:\/\//i.test(u)) return { state: "invalid", message: `Use https://, not http://. ${REPO_URL_HELP}` };
  return { state: "invalid", message: `Link to the repository itself, not a page inside it. ${REPO_URL_HELP}` };
}
