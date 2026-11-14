"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { NewSpaceDialog } from "./new-space-dialog";

/** The New space dialog as a page of its own: closing it goes back to where it was opened from. */
export function NewSpaceRoute() {
  const router = useRouter();
  const work = useSearchParams().get("work") === "1";
  return <NewSpaceDialog open defaultTracking={work} onOpenChange={(o) => !o && router.back()} />;
}
