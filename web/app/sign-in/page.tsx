import { Suspense } from "react";
import { EmptyState } from "@/components/ui/primitives";

async function Message({ searchParams }: { searchParams: PageProps<"/sign-in">["searchParams"] }) {
  const { error } = await searchParams;
  return (
    <EmptyState icon="lock" title={error ? "That sign-in link didn't work" : "Sign in to Vesper"}>
      {error ? "It was already used or has expired (links last 15 minutes). " : ""}
      Run <code className="font-mono">make signin</code> where Vesper runs and open the link it prints. Each link works
      once and keeps this browser signed in for a year.
    </EmptyState>
  );
}

/**
 * Where the app sends a browser the API refused. Sign-in links come from a
 * terminal, never from the app: agent containers can reach everything the
 * app serves, so nothing served here may hand out a session.
 */
export default function SignInPage({ searchParams }: PageProps<"/sign-in">) {
  return (
    <main className="grid min-h-dvh place-items-center">
      <Suspense>
        <Message searchParams={searchParams} />
      </Suspense>
    </main>
  );
}
