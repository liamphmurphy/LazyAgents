"use client";

import { useState, type FormEvent } from "react";
import { sendCredentials } from "@/lib/owner-sign-in";
import { Button } from "@/components/ui/button";
import { Field, TextInput } from "@/components/ui/primitives";

/**
 * Username and password, either chosen (action "setup": with a confirmation,
 * matching the server's rules) or entered (action "login"). onDone runs once
 * the API accepted them.
 */
export function OwnerCredentialsForm({
  action,
  submitLabel,
  autoFocus = false,
  onDone,
}: {
  action: "setup" | "login";
  submitLabel: string;
  autoFocus?: boolean;
  onDone: () => void;
}) {
  const choosing = action === "setup";
  const [login, setLogin] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState<{ message: string; field?: "login" | "password" | "confirm" } | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (choosing && password !== confirm) {
      setError({ message: "The passwords don't match.", field: "confirm" });
      return;
    }
    setBusy(true);
    setError(null);
    const r = await sendCredentials(action, login, password);
    if (r.ok) {
      onDone();
      return;
    }
    setBusy(false);
    setError({ message: r.message, field: r.field });
  }

  const invalid = (f: "login" | "password" | "confirm") => error?.field === f || undefined;
  return (
    <form onSubmit={submit} className="flex w-full flex-col gap-3 text-left" noValidate>
      <Field label="Username" htmlFor="owner-login" hint={choosing ? "Letters, digits, or . _ @ -" : undefined}>
        <TextInput
          id="owner-login"
          name="username"
          autoComplete="username"
          autoCapitalize="none"
          spellCheck={false}
          required
          autoFocus={autoFocus}
          value={login}
          onChange={(e) => setLogin(e.target.value)}
          aria-invalid={invalid("login")}
        />
      </Field>
      <Field label="Password" htmlFor="owner-password" hint={choosing ? "At least 8 characters." : undefined}>
        <TextInput
          id="owner-password"
          name="password"
          type="password"
          autoComplete={choosing ? "new-password" : "current-password"}
          required
          minLength={choosing ? 8 : undefined}
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          aria-invalid={invalid("password")}
        />
      </Field>
      {choosing && (
        <Field label="Confirm password" htmlFor="owner-confirm">
          <TextInput
            id="owner-confirm"
            name="confirm"
            type="password"
            autoComplete="new-password"
            required
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
            aria-invalid={invalid("confirm")}
          />
        </Field>
      )}
      {error && (
        <p role="alert" className="text-[12.5px] text-pill-blocked-fg">
          {error.message}
        </p>
      )}
      <Button type="submit" variant="primary" disabled={busy || !login || !password} className="justify-center">
        {busy ? "One moment…" : submitLabel}
      </Button>
    </form>
  );
}
