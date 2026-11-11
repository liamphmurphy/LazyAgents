/**
 * Password sign-in for the instance's owner. These routes answer browsers
 * that aren't signed in yet, so they sit beside the typed API client rather
 * than in it: its 401 handler would send the sign-in page to itself.
 */

export type AuthStatus = { claimed: boolean; password_set: boolean; signed_in: boolean };

export type CredentialsResult = { ok: true } | { ok: false; message: string; field?: "login" | "password" };

const base = "/api/v1/auth";

export async function fetchAuthStatus(): Promise<AuthStatus> {
  const r = await fetch(`${base}/status`, { cache: "no-store" });
  if (!r.ok) throw new Error(`auth status: ${r.status}`);
  return r.json();
}

/** setup claims a fresh install (or adds a password while signed in); login signs in with one. */
export async function sendCredentials(action: "setup" | "login", login: string, password: string): Promise<CredentialsResult> {
  let r: Response;
  try {
    r = await fetch(`${base}/${action}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ login, password }),
    });
  } catch {
    return { ok: false, message: "Couldn't reach Vesper. Is the API running?" };
  }
  if (r.ok) return { ok: true };
  const e = (await r.json().catch(() => ({}))) as { message?: string; field?: string };
  const field = e.field === "login" || e.field === "password" ? e.field : undefined;
  return { ok: false, message: e.message ?? `Something went wrong (${r.status}).`, field };
}
