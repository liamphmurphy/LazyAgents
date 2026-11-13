"use client";

import { useEffect, useRef, useState } from "react";
import { useBootstrap } from "@/lib/api/hooks";
import { useDeleteProfileAvatar, useUpdateProfile, useUploadProfileAvatar } from "@/lib/api/profile-hooks";
import { AVATAR_ACCEPT, AVATAR_HINT, avatarUploadError, checkAvatarFile, fieldError, type AvatarError } from "@/lib/persona-ui";
import { EMAILS_MAX, NAME_MAX, ROLE_MAX, emailError, makePrimary, profileChanges, validateProfile } from "@/lib/profile-ui";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { Banner, Card, Field, Skeleton, TextInput } from "@/components/ui/primitives";
import { HumanAvatar } from "@/components/domain/avatars";

/** Customization → You: your picture, display name, role title and email addresses. */
export function ProfileSection() {
  const boot = useBootstrap();
  if (boot.isError) {
    return (
      <Banner tone="blocked" title="Couldn’t load your profile" actions={<Button variant="outline" onClick={() => void boot.refetch()}>Retry</Button>}>
        {boot.error.message || "The API is unavailable."}
      </Banner>
    );
  }
  if (!boot.data) return <Skeleton aria-label="Loading profile" className="h-40" />;
  const me = boot.data.me;
  return (
    <div className="flex flex-col gap-4">
      <PictureCard name={me.name} initials={me.initials} color={me.avatar_color} src={me.avatar_url} />
      <DetailsCard name={me.name} roleTitle={me.role_title} emails={me.emails ?? [me.email]} />
    </div>
  );
}

function PictureCard({ name, initials, color, src }: { name: string; initials: string; color?: string; src?: string }) {
  const input = useRef<HTMLInputElement>(null);
  const upload = useUploadProfileAvatar();
  const remove = useDeleteProfileAvatar();
  const [error, setError] = useState<AvatarError | null>(null);
  const busy = upload.isPending || remove.isPending;

  function take(files: FileList | null) {
    const f = files?.[0];
    if (!f) return;
    const bad = checkAvatarFile(f);
    if (bad) return setError(bad);
    setError(null);
    upload.mutate(f, { onError: (err) => setError(avatarUploadError(err, f)) });
  }

  return (
    <Card title="Profile picture">
      <div className="flex flex-wrap items-center gap-5">
        <HumanAvatar name={name} initials={initials} color={color} src={src} size={72} />
        <div className="flex flex-col gap-2">
          <div className="flex flex-wrap items-center gap-2">
            <input ref={input} type="file" accept={AVATAR_ACCEPT} hidden aria-label="Profile picture file" onChange={(e) => { take(e.target.files); e.target.value = ""; }} />
            <Button variant="secondary" icon="upload" disabled={busy} onClick={() => input.current?.click()}>
              {upload.isPending ? "Uploading…" : src ? "Replace picture" : "Upload picture"}
            </Button>
            {src && (
              <Button variant="ghost" disabled={busy} onClick={() => { setError(null); remove.mutate(undefined, { onError: (e) => setError({ title: "Couldn’t remove the picture.", hint: e instanceof Error ? e.message : "Please try again." }) }); }}>
                Remove
              </Button>
            )}
          </div>
          <p className="text-12 text-fg-tertiary">{AVATAR_HINT} Without one, your initials show.</p>
        </div>
      </div>
      <div aria-live="polite">
        {error && (
          <div role="alert" className="mt-3 flex items-start gap-2 rounded-lg bg-pill-blocked-bg px-3 py-2">
            <div className="flex min-w-0 flex-1 flex-col">
              <span className="text-12 text-pill-blocked-fg">{error.title}</span>
              <span className="text-12 text-fg-secondary">{error.hint}</span>
            </div>
            <button type="button" aria-label="Dismiss" className="text-12 text-pill-blocked-fg" onClick={() => setError(null)}>✕</button>
          </div>
        )}
      </div>
    </Card>
  );
}

function DetailsCard({ name, roleTitle, emails }: { name: string; roleTitle: string; emails: string[] }) {
  const update = useUpdateProfile();
  const [draft, setDraft] = useState({ name, role_title: roleTitle, emails });
  const [notice, setNotice] = useState("");
  useEffect(() => {
    if (!notice) return;
    const t = setTimeout(() => setNotice(""), 6000);
    return () => clearTimeout(t);
  }, [notice]);

  const local = validateProfile(draft);
  const changes = profileChanges({ name, role_title: roleTitle, emails }, draft);
  const server = fieldError(update.error);
  const nameError = local.name ?? (server?.field === "name" ? server.message : undefined);
  const roleError = local.role_title ?? (server?.field === "role_title" ? server.message : undefined);
  const emailsError = local.emails ?? (server?.field === "emails" ? server.message : undefined);
  const invalid = !!local.name || !!local.role_title || !!local.emails;
  const otherError = update.error && !server ? update.error.message || "Couldn’t save your profile." : null;
  const edit = (patch: Partial<typeof draft>) => {
    update.reset();
    setNotice("");
    setDraft({ ...draft, ...patch });
  };

  return (
    <Card title="Details">
      <form
        className="flex flex-col gap-3"
        onSubmit={(e) => {
          e.preventDefault();
          if (!changes || invalid) return;
          update.mutate(changes, { onSuccess: () => setNotice("Profile saved.") });
        }}
      >
        <Field label="Display name" htmlFor="profile-name" hint={nameError ? <span role="alert" className="text-pill-blocked-fg">{nameError}</span> : `Shown to teammates, up to ${NAME_MAX} characters.`}>
          <TextInput id="profile-name" value={draft.name} required autoComplete="name" aria-invalid={!!nameError} aria-describedby="profile-name-hint" onChange={(e) => edit({ name: e.target.value })} />
        </Field>
        <Field label="Role title" htmlFor="profile-role" hint={roleError ? <span role="alert" className="text-pill-blocked-fg">{roleError}</span> : `Optional, up to ${ROLE_MAX} characters.`}>
          <TextInput id="profile-role" value={draft.role_title} autoComplete="organization-title" aria-invalid={!!roleError} aria-describedby="profile-role-hint" onChange={(e) => edit({ role_title: e.target.value })} />
        </Field>
        <EmailList emails={draft.emails} error={emailsError} onChange={(next) => edit({ emails: next })} />
        {otherError && <p role="alert" className="text-12 text-pill-blocked-fg">{otherError}</p>}
        <div className="flex flex-wrap items-center gap-3">
          <Button type="submit" variant="primary" disabled={!changes || invalid || update.isPending}>
            {update.isPending ? "Saving…" : "Save profile"}
          </Button>
          {notice && (
            <p role="status" className="flex items-center gap-1.5 text-13 text-pill-done-fg">
              <Icon name="check" size={13} /> {notice}
            </p>
          )}
        </div>
      </form>
    </Card>
  );
}

/**
 * Your addresses, primary first. Coding chats commit as the primary when it's
 * the only one; with several, each coding chat asks which to use.
 */
function EmailList({ emails, error, onChange }: { emails: string[]; error?: string; onChange: (next: string[]) => void }) {
  const [adding, setAdding] = useState("");
  const [addError, setAddError] = useState<string>();
  const add = () => {
    const bad = emailError(adding, emails);
    if (bad) return setAddError(bad);
    onChange([...emails, adding.trim()]);
    setAdding("");
    setAddError(undefined);
  };
  const hint = error ? (
    <span role="alert" className="text-pill-blocked-fg">{error}</span>
  ) : emails.length > 1 ? (
    "The first is your primary. Each coding chat asks which one its commits use."
  ) : (
    "Commits in coding chats are authored with this address. Add another to choose per chat."
  );
  return (
    <Field label="Email addresses" htmlFor="profile-email-new" hint={hint}>
      <ul aria-label="Email addresses" className="flex flex-col divide-y divide-border overflow-hidden rounded-md border border-border-strong">
        {emails.map((e, i) => (
          <li key={e} className="flex min-w-0 items-center gap-2 bg-surface px-3 py-1.5">
            <Icon name="at" size={13} className="flex-none text-fg-tertiary" />
            <span className="min-w-0 flex-1 truncate text-13 text-fg" title={e}>{e}</span>
            {i === 0 ? (
              <span className="flex-none rounded-full bg-selected px-2 py-0.5 text-11 font-medium text-fg-secondary">Primary</span>
            ) : (
              <button type="button" className="flex-none text-12 text-accent hover:underline" onClick={() => onChange(makePrimary(emails, e))}>
                Make primary
              </button>
            )}
            <button
              type="button"
              aria-label={`Remove ${e}`}
              title={emails.length === 1 ? "Keep at least one address" : `Remove ${e}`}
              disabled={emails.length === 1}
              className="flex-none rounded p-1 text-fg-tertiary hover:bg-hover hover:text-fg disabled:cursor-not-allowed disabled:opacity-40"
              onClick={() => onChange(emails.filter((x) => x !== e))}
            >
              <Icon name="x" size={12} />
            </button>
          </li>
        ))}
      </ul>
      {emails.length < EMAILS_MAX && (
        <div className="flex items-start gap-2">
          <div className="flex min-w-0 flex-1 flex-col gap-1">
            <TextInput
              id="profile-email-new"
              type="email"
              value={adding}
              placeholder="Add another address"
              autoComplete="email"
              aria-invalid={!!addError}
              className="flex-1"
              onChange={(ev) => {
                setAdding(ev.target.value);
                setAddError(undefined);
              }}
              onKeyDown={(ev) => {
                // Enter adds the address rather than saving the whole form.
                if (ev.key === "Enter" && !ev.nativeEvent.isComposing) {
                  ev.preventDefault();
                  add();
                }
              }}
            />
            {addError && <span role="alert" className="text-12 text-pill-blocked-fg">{addError}</span>}
          </div>
          <Button variant="secondary" icon="plus" disabled={!adding.trim()} onClick={add}>
            Add
          </Button>
        </div>
      )}
    </Field>
  );
}
