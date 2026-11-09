// Pure helpers for the Settings → Profile form.

export const NAME_MAX = 80;
export const ROLE_MAX = 80;
export const EMAILS_MAX = 10;
const EMAIL_LENGTH_MAX = 254;

/** emails: every address, the first the primary. */
export type ProfileDraft = { name: string; role_title: string; emails: string[] };
export type ProfileErrors = { name?: string; role_title?: string; emails?: string };

// A light check; the server parses each address properly.
const EMAIL_SHAPE = /^[^\s@<>()",;:]+@[^\s@<>()",;:]+$/;

/** Why `value` can't be added to `existing`, or undefined when it can. */
export function emailError(value: string, existing: string[]): string | undefined {
  const v = value.trim();
  if (!v) return "Enter an email address.";
  if (v.length > EMAIL_LENGTH_MAX || !EMAIL_SHAPE.test(v)) return "That doesn’t look like an email address.";
  if (existing.some((e) => e.toLowerCase() === v.toLowerCase())) return "That address is already listed.";
  if (existing.length >= EMAILS_MAX) return `Up to ${EMAILS_MAX} addresses.`;
  return undefined;
}

/** Client-side limits, mirroring the server's (trimmed name 1–80, role title ≤ 80, 1–10 addresses). */
export function validateProfile(d: ProfileDraft): ProfileErrors {
  const e: ProfileErrors = {};
  const name = [...d.name.trim()].length;
  if (name === 0) e.name = "Enter a display name.";
  else if (name > NAME_MAX) e.name = `Use ${NAME_MAX} characters or fewer.`;
  if ([...d.role_title.trim()].length > ROLE_MAX) e.role_title = `Use ${ROLE_MAX} characters or fewer.`;
  if (d.emails.length === 0) e.emails = "Keep at least one email address.";
  return e;
}

/** Only the changed, trimmed fields, or null when nothing differs from the saved profile. */
export function profileChanges(saved: ProfileDraft, d: ProfileDraft): Partial<ProfileDraft> | null {
  const out: Partial<ProfileDraft> = {};
  if (d.name.trim() !== saved.name) out.name = d.name.trim();
  if (d.role_title.trim() !== saved.role_title) out.role_title = d.role_title.trim();
  const emails = d.emails.map((e) => e.trim());
  if (emails.join("\n") !== saved.emails.join("\n")) out.emails = emails;
  return Object.keys(out).length ? out : null;
}

/** `emails` with `email` moved to the front, making it the primary. */
export function makePrimary(emails: string[], email: string): string[] {
  return [email, ...emails.filter((e) => e !== email)];
}
