"use client";

import { useBootstrap } from "@/lib/api/hooks";
import { Skeleton } from "@/components/ui/primitives";

function partOfDay(h: number) {
  return h < 12 ? "morning" : h < 18 ? "afternoon" : "evening";
}

export function Greeting() {
  const { data } = useBootstrap();
  if (!data) return <Skeleton className="h-7 w-64" />;
  return (
    <h1 className="type-display">
      Good {partOfDay(new Date().getHours())}, {data.me.name.split(" ")[0]}
    </h1>
  );
}
