"use client";

import { useRouter } from "next/navigation";
import { useEffect, useState, type ReactNode } from "react";
import { resolveLegacy } from "@/lib/api/knowledge-hooks";
import { Skeleton } from "@/components/ui/primitives";

/**
 * An address from before Docs and the wiki merged: /apps/docs/<id> or
 * /wiki/<path>. It goes to the page that holds it now. When nothing does
 * (a canvas that was never a Doc, a path that never existed), the old
 * screen still shows, so no bookmark becomes a dead end.
 */
export function LegacyRedirect({ from, fallback }: { from: string; fallback: ReactNode }) {
  const router = useRouter();
  const [missing, setMissing] = useState(false);
  useEffect(() => {
    let live = true;
    resolveLegacy(from).then(
      (r) => live && router.replace(r.url),
      () => live && setMissing(true),
    );
    return () => {
      live = false;
    };
  }, [from, router]);
  if (missing) return <>{fallback}</>;
  return (
    <div aria-busy="true" className="mx-auto w-full max-w-[680px] space-y-4 px-6 pt-16">
      <Skeleton className="h-9 w-1/2" />
      <Skeleton className="h-4 w-full" />
    </div>
  );
}
