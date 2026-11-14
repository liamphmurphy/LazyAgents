"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useEffect } from "react";
import { useBootstrap } from "@/lib/api/hooks";
import { workSettingsHref } from "@/lib/spaces";
import { LoadingLines } from "@/components/ui/loading";

/** Settings › Projects is gone: ?project=KEY opens that space's page, anything else a new space that tracks work. */
export function ProjectsRedirect() {
  const router = useRouter();
  const key = useSearchParams().get("project");
  const boot = useBootstrap();
  const projects = boot.data?.projects;
  useEffect(() => {
    if (!projects && !boot.isError) return;
    router.replace(workSettingsHref(projects?.find((p) => p.key === key)));
  }, [projects, boot.isError, key, router]);
  return (
    <div className="p-[40px]">
      <LoadingLines label="Opening the space" />
    </div>
  );
}
