"use client";

import { useBootstrap } from "@/lib/api/hooks";
import { needsProfileSetup } from "@/lib/profile-ui";
import { setStoredFlag, setupIntroKey, SetupChecklist } from "@/components/domain/setup";
import { ButtonLink } from "@/components/ui/button";
import { ProfileSection } from "../customization/profile-section";
import { PageHeader } from "@/components/ui/page-header";

/** Reuses the profile editor and live checklist rather than maintaining a second set of settings. */
export default function Setup() {
  const boot = useBootstrap();
  const ready = boot.data && !needsProfileSetup(boot.data.me);
  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-6 p-6">
      <PageHeader title="Get started with Vesper" description="Set your display name and primary email first. Agents author commits with your primary email; coding chats can use any address in your profile." />
      <ProfileSection />
      <p className="text-14 text-fg-secondary">Your Assistant starts with all persona permissions enabled. Connect a provider to chat; repositories and agents can be added whenever you need them.</p>
      <SetupChecklist />
      {ready ? <ButtonLink href="/chat?new" variant="primary" className="self-start" onClick={() => setStoredFlag("local", setupIntroKey(boot.data.workspace.id))}>Continue to chat</ButtonLink> : <p role="status" className="text-14 text-fg-secondary">Replace the placeholder name and primary email, then save your profile to continue.</p>}
    </div>
  );
}
