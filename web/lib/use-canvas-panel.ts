"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";

/** The canvas pane beside a chat, in the URL like Memory and Personas. */
export function useCanvasPanel() {
  const router = useRouter();
  const pathname = usePathname();
  const search = useSearchParams();
  const raw = search.get("canvas");
  const go = (value: string | null) => {
    const q = new URLSearchParams(search.toString());
    if (value === null) q.delete("canvas");
    else {
      q.set("canvas", value);
      // One right-hand panel at a time.
      q.delete("memory");
      q.delete("personas");
    }
    router.replace(`${pathname}?${q.toString()}`, { scroll: false });
  };
  return {
    open: raw !== null,
    focus: raw && raw !== "1" ? raw : undefined,
    show: (id?: string) => go(id ?? "1"),
    close: () => go(null),
  };
}
