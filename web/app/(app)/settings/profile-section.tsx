"use client";

import { useEffect, useRef, useState } from "react";
import { useBootstrap } from "@/lib/api/hooks";
import { useDeleteProfileAvatar, useUpdateProfile, useUploadProfileAvatar } from "@/lib/api/profile-hooks";
import { AVATAR_ACCEPT, AVATAR_HINT, avatarUploadError, checkAvatarFile, fieldError, type AvatarError } from "@/lib/persona-ui";
import { NAME_MAX, ROLE_MAX, profileChanges, validateProfile } from "@/lib/profile-ui";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { Banner, Card, Field, Skeleton, TextInput } from "@/components/ui/primitives";
import { HumanAvatar } from "@/components/domain/avatars";

/** Settings → Profile: your picture, display name, role title and (read-only) email. */
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
      <DetailsCard name={me.name} roleTitle={me.role_title} email={me.email} />
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
          <p className="text-[11.5px] text-fg-tertiary">{AVATAR_HINT} Without one, your initials show.</p>
        </div>
      </div>
      <div aria-live="polite">
        {error && (
          <div role="alert" className="mt-3 flex items-start gap-2 rounded-lg bg-pill-blocked-bg px-3 py-2">
            <div className="flex min-w-0 flex-1 flex-col">
              <span className="text-xs text-pill-blocked-fg">{error.title}</span>
              <span className="text-xs text-fg-secondary">{error.hint}</span>
            </div>
            <button type="button" aria-label="Dismiss" className="text-xs text-pill-blocked-fg" onClick={() => setError(null)}>✕</button>
          </div>
        )}
      </div>
    </Card>
  );
}

function DetailsCard({ name, roleTitle, email }: { name: string; roleTitle: string; email: string }) {
  const update = useUpdateProfile();
  const [draft, setDraft] = useState({ name, role_title: roleTitle });
  const [notice, setNotice] = useState("");
  useEffect(() => {
    if (!notice) return;
    const t = setTimeout(() => setNotice(""), 6000);
    return () => clearTimeout(t);
  }, [notice]);

  const local = validateProfile(draft);
  const changes = profileChanges({ name, role_title: roleTitle }, draft);
  const server = fieldError(update.error);
  const nameError = local.name ?? (server?.field === "name" ? server.message : undefined);
  const roleError = local.role_title ?? (server?.field === "role_title" ? server.message : undefined);
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
          if (!changes || local.name || local.role_title) return;
          update.mutate(changes, { onSuccess: () => setNotice("Profile saved.") });
        }}
      >
        <Field label="Display name" htmlFor="profile-name" hint={nameError ? <span role="alert" className="text-pill-blocked-fg">{nameError}</span> : `Shown to teammates, up to ${NAME_MAX} characters.`}>
          <TextInput id="profile-name" value={draft.name} required autoComplete="name" aria-invalid={!!nameError} aria-describedby="profile-name-hint" onChange={(e) => edit({ name: e.target.value })} />
        </Field>
        <Field label="Role title" htmlFor="profile-role" hint={roleError ? <span role="alert" className="text-pill-blocked-fg">{roleError}</span> : `Optional, up to ${ROLE_MAX} characters.`}>
          <TextInput id="profile-role" value={draft.role_title} autoComplete="organization-title" aria-invalid={!!roleError} aria-describedby="profile-role-hint" onChange={(e) => edit({ role_title: e.target.value })} />
        </Field>
        <Field label="Email" htmlFor="profile-email" hint="Your sign-in address. It can’t be changed here.">
          <TextInput id="profile-email" value={email} readOnly disabled aria-describedby="profile-email-hint" />
        </Field>
        {otherError && <p role="alert" className="text-xs text-pill-blocked-fg">{otherError}</p>}
        <div className="flex flex-wrap items-center gap-3">
          <Button type="submit" variant="primary" disabled={!changes || !!local.name || !!local.role_title || update.isPending}>
            {update.isPending ? "Saving…" : "Save profile"}
          </Button>
          {notice && (
            <p role="status" className="flex items-center gap-1.5 text-[13px] text-pill-done-fg">
              <Icon name="check" size={13} /> {notice}
            </p>
          )}
        </div>
      </form>
    </Card>
  );
}
