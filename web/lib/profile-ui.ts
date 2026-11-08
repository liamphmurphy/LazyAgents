// Pure helpers for the Settings → Profile form.

export const NAME_MAX = 80;
export const ROLE_MAX = 80;

export type ProfileDraft = { name: string; role_title: string };
export type ProfileErrors = { name?: string; role_title?: string };

/** Client-side limits, mirroring the server's (trimmed name 1–80, role title ≤ 80). */
export function validateProfile(d: ProfileDraft): ProfileErrors {
  const e: ProfileErrors = {};
  const name = [...d.name.trim()].length;
  if (name === 0) e.name = "Enter a display name.";
  else if (name > NAME_MAX) e.name = `Use ${NAME_MAX} characters or fewer.`;
  if ([...d.role_title.trim()].length > ROLE_MAX) e.role_title = `Use ${ROLE_MAX} characters or fewer.`;
  return e;
}

/** Only the changed, trimmed fields, or null when nothing differs from the saved profile. */
export function profileChanges(saved: ProfileDraft, d: ProfileDraft): Partial<ProfileDraft> | null {
  const out: Partial<ProfileDraft> = {};
  if (d.name.trim() !== saved.name) out.name = d.name.trim();
  if (d.role_title.trim() !== saved.role_title) out.role_title = d.role_title.trim();
  return Object.keys(out).length ? out : null;
}
