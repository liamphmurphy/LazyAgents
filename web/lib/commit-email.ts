// Which of your addresses a coding chat's commits are authored as. With one
// address there's nothing to ask; with several, each coding chat asks.

/** Whether a chat in `repo` should ask which of `emails` to commit as: it has a repository, you have a choice, and nothing valid is picked. */
export function needsCommitEmailPick(emails: string[], repo: string | undefined, picked: string | undefined): boolean {
  if (!repo || emails.length < 2) return false;
  return !picked || !emails.some((e) => e.toLowerCase() === picked.toLowerCase());
}

/** A stable, made-up short hash for the picker's commit preview, so each address gets its own. */
export function previewHash(email: string): string {
  let h = 0x811c9dc5;
  for (const ch of email.toLowerCase()) {
    h ^= ch.codePointAt(0)!;
    h = Math.imul(h, 0x01000193) >>> 0;
  }
  return h.toString(16).padStart(8, "0").slice(0, 7);
}
