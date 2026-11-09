"use client";

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useEffect, useLayoutEffect, useRef, useSyncExternalStore } from "react";
import { useArtifact, useArtifactVersionBody } from "@/lib/api/hooks";
import { absoluteTime } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { Banner, Skeleton } from "@/components/ui/primitives";
import { artifactTitle } from "@/components/domain/artifacts";
import { Markdown } from "@/components/domain/markdown";
import { PrintMode } from "@/components/domain/print-mode";

/** Longest wait for diagrams, fonts and images before printing anyway. */
const READY_TIMEOUT_MS = 15_000;

function subscribeTheme(onChange: () => void) {
  const attr = new MutationObserver(onChange);
  attr.observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme"] });
  return () => attr.disconnect();
}

/**
 * Paper is white: print in the light theme whatever the app uses, and put it back after.
 * True once it applies, so visuals that read the theme once (Mermaid) draw light.
 */
function useLightTheme() {
  useLayoutEffect(() => {
    const root = document.documentElement;
    const prev = root.getAttribute("data-theme");
    root.setAttribute("data-theme", "light");
    return () => {
      if (prev) root.setAttribute("data-theme", prev);
      else root.removeAttribute("data-theme");
    };
  }, []);
  return useSyncExternalStore(subscribeTheme, () => document.documentElement.getAttribute("data-theme") === "light", () => false);
}

/** Resolves once Mermaid has drawn, fonts have loaded and every image has decoded. */
async function settled(root: HTMLElement) {
  const deadline = Date.now() + READY_TIMEOUT_MS;
  const frame = () => new Promise((r) => requestAnimationFrame(r));
  while (Date.now() < deadline && root.querySelector('[data-testid="mermaid-loading"]')) {
    await new Promise((r) => setTimeout(r, 100));
  }
  await document.fonts.ready;
  await Promise.all(
    Array.from(root.querySelectorAll("img")).map((img) => img.decode().catch(() => undefined)),
  );
  await frame();
  await frame();
}

export function PrintArtifact({ id }: { id: string }) {
  const themed = useLightTheme();
  const versionId = useSearchParams().get("version");
  const { data: a, isError, error } = useArtifact(id);
  const version = a?.versions.find((v) => v.id === versionId) ?? a?.current_version;
  const isOld = !!a && !!version && version.id !== a.current_version.id;
  const old = useArtifactVersionBody(a, versionId);
  const body = isOld ? old.data : a?.body_md;
  const printable = !!version && version.content.kind === "markdown" && a?.type !== "mockup";
  // Bodies often open with their own "# Title"; don't print it twice.
  const ownTitle = !!a && !!body && /^\s*#\s+(.+)/.exec(body)?.[1].trim() === a.title.trim();
  const ref = useRef<HTMLElement>(null);
  const printed = useRef(false);

  // The PDF's default file name is the page title.
  useEffect(() => {
    if (a) document.title = `${artifactTitle(a)}${isOld && version ? ` (${version.label})` : ""}`;
  }, [a, isOld, version]);

  // Open the print dialog once, when everything on the page has drawn.
  useEffect(() => {
    if (!themed || !printable || body == null || printed.current || !ref.current) return;
    printed.current = true;
    let live = true;
    settled(ref.current).then(() => {
      if (live) window.print();
    });
    return () => {
      live = false;
      printed.current = false;
    };
  }, [themed, printable, body]);

  if (isError) {
    return (
      <div className="mx-auto max-w-[720px] p-8">
        <Banner tone="blocked" title="Couldn't load this artifact">{error.message}</Banner>
      </div>
    );
  }

  return (
    <div className="min-h-dvh bg-surface text-fg print:bg-transparent">
      <div className="sticky top-0 z-10 flex items-center gap-2 border-b border-border-subtle bg-panel px-6 py-2 print:hidden">
        <Link href={`/artifacts/${id}${isOld && version ? `?version=${version.id}` : ""}`} className="text-[13px] text-fg-secondary hover:text-fg">
          ← Back to artifact
        </Link>
        <span className="ml-auto text-xs text-fg-tertiary">Choose “Save as PDF” as the destination.</span>
        <Button variant="primary" icon="printer" disabled={!printable || body == null} onClick={() => window.print()}>
          Print / Save as PDF
        </Button>
      </div>
      <article ref={ref} data-testid="print-artifact" className="print-doc mx-auto flex w-full max-w-[760px] flex-col gap-5 px-8 py-10 print:max-w-none print:p-0">
        {!themed || !a || !version || (printable && body == null) ? (
          <div aria-busy="true" className="flex flex-col gap-4">
            <Skeleton className="h-7 w-2/3" />
            <Skeleton className="h-3 w-full" />
            <Skeleton className="h-3 w-5/6" />
          </div>
        ) : !printable ? (
          <Banner tone="human" title="Only markdown artifacts export to PDF">
            Use Download on the artifact page for files and mockups.
          </Banner>
        ) : (
          <>
            <header className="flex flex-col gap-2 border-b border-border-subtle pb-5">
              {!ownTitle && <h1 className="type-display">{artifactTitle(a)}</h1>}
              <p className="font-mono text-xs text-fg-tertiary">
                {version.label} · {version.created_by.name} · {absoluteTime(version.created_at)}
              </p>
              {a.summary && <p className="text-sm leading-relaxed text-fg-secondary">{a.summary}</p>}
            </header>
            <PrintMode.Provider value={true}>
              <Markdown anchors visualBlocks className="type-reading text-fg-2">{body!}</Markdown>
            </PrintMode.Provider>
          </>
        )}
      </article>
    </div>
  );
}
