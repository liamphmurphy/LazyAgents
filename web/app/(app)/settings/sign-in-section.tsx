"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { fetchAuthStatus } from "@/lib/owner-sign-in";
import { OwnerCredentialsForm } from "@/components/domain/owner-sign-in-form";
import { Card, Skeleton } from "@/components/ui/primitives";

const authStatusKey = ["auth-status"];

export function useAuthStatus() {
  return useQuery({ queryKey: authStatusKey, queryFn: fetchAuthStatus });
}

/**
 * The owner's password. Installs set up before passwords existed were opened
 * with a sign-in link; this is where they add one for other devices.
 */
export function SignInSection() {
  const status = useAuthStatus();
  const qc = useQueryClient();
  if (status.isPending) return <Skeleton className="h-24 w-full" />;
  if (status.isError) return <p className="text-13 text-fg-secondary">Couldn&apos;t load sign-in settings.</p>;
  if (status.data.password_set) {
    return (
      <Card className="p-4 text-13 text-fg-2">
        Other devices sign in with your username and password. Forgot it? Run <code className="font-mono">make signin</code> where
        Vesper runs for a one-time link.
      </Card>
    );
  }
  return (
    <Card className="flex max-w-[400px] flex-col gap-3 p-4">
      <p className="text-13 text-fg-2">
        This browser was signed in with a link. Choose a username and password to sign in from other devices without a terminal.
      </p>
      <OwnerCredentialsForm action="setup" submitLabel="Save password" onDone={() => qc.invalidateQueries({ queryKey: authStatusKey })} />
    </Card>
  );
}
