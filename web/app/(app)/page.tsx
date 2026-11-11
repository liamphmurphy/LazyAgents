"use client";

import { useRouter } from "next/navigation";
import { useEffect } from "react";
import { usePersonas } from "@/lib/api/persona-hooks";
import { mostUsedPersona } from "@/lib/persona-ui";
import { Skeleton } from "@/components/ui/primitives";
import { useBootstrap } from "@/lib/api/hooks";
import { needsWizard } from "@/lib/onboarding";

/** First run opens the Welcome Wizard (in the new-chat welcome), otherwise a new chat with the persona the person chats with most. */
export default function Landing() {
  const router = useRouter();
  const personas = usePersonas();
  const boot = useBootstrap();
  // A failed lookup still lands in a new chat, which falls back to the default persona.
  const settled = !personas.isPending;
  useEffect(() => {
    if (!settled || !boot.data) return;
    if (needsWizard(boot.data.me)) {
      router.replace("/chat?new");
      return;
    }
    const persona = mostUsedPersona(personas.data?.personas, personas.data?.default_persona_id);
    router.replace(persona ? `/chat?new&persona=${encodeURIComponent(persona)}` : "/chat?new");
  }, [settled, personas.data, boot.data, router]);
  return (
    <div aria-busy="true" className="flex flex-1 flex-col gap-4 p-6">
      <Skeleton className="h-6 w-1/3" />
      <Skeleton className="h-16 w-2/3" />
    </div>
  );
}
