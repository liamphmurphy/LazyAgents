"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { APP_RESIZE, appDocument, appHeight } from "@/lib/chat-app";
import { cx } from "@/lib/cx";
import { Icon } from "@/components/ui/icon";
import { CodeFence } from "./code-fence";

/**
 * An ```app fence in chat: the assistant's HTML/JS runs live in a sandboxed
 * iframe. `allow-scripts` without `allow-same-origin` gives the frame an
 * opaque origin, so it can't read Vesper's cookies, storage or DOM; the CSP in
 * appDocument keeps it off the network. Code shows the source; Reload restarts it.
 */
export function AppFence({ code, info }: { code: string; info?: string }) {
  const [view, setView] = useState<"app" | "code">("app");
  const [run, setRun] = useState(0);
  const [height, setHeight] = useState(appHeight(0));
  const frame = useRef<HTMLIFrameElement>(null);
  const doc = useMemo(() => appDocument(code), [code]);

  useEffect(() => {
    const onMessage = (e: MessageEvent) => {
      // Only this frame's own window: the opaque origin is "null", so match on source.
      if (!frame.current || e.source !== frame.current.contentWindow) return;
      const d = e.data as { type?: unknown; height?: unknown } | null;
      if (d && d.type === APP_RESIZE) setHeight(appHeight(d.height));
    };
    window.addEventListener("message", onMessage);
    return () => window.removeEventListener("message", onMessage);
  }, []);

  const tab = (v: "app" | "code", label: string) => (
    <button
      type="button"
      role="tab"
      aria-selected={view === v}
      className={cx("rounded px-1.5", view === v ? "bg-surface text-fg" : "hover:text-fg")}
      onClick={() => setView(v)}
    >
      {label}
    </button>
  );

  return (
    <div className="overflow-hidden rounded-lg border border-border bg-surface-sunken" data-testid="app-fence">
      <div className="flex items-center justify-between gap-2 border-b border-border-row px-3 py-1 text-xs text-fg-tertiary">
        <span className="flex items-center gap-1.5">
          <Icon name="play" size={12} />
          <span>Interactive app</span>
        </span>
        <span className="flex items-center gap-1">
          <span role="tablist" aria-label="App view" className="flex items-center gap-0.5">
            {tab("app", "App")}
            {tab("code", "Code")}
          </span>
          {view === "app" && (
            <button type="button" aria-label="Reload app" title="Reload" className="flex items-center rounded px-1 hover:text-fg" onClick={() => setRun((r) => r + 1)}>
              <Icon name="refresh" size={12} />
            </button>
          )}
        </span>
      </div>
      {view === "app" ? (
        <iframe
          key={run}
          ref={frame}
          title="Interactive app"
          sandbox="allow-scripts"
          referrerPolicy="no-referrer"
          srcDoc={doc}
          style={{ height }}
          className="block w-full border-0 bg-white"
        />
      ) : (
        // The inner fence sits flush inside this frame's border.
        <div className="[&>div]:rounded-none [&>div]:border-0">
          <CodeFence code={code} info={info?.replace(/^\s*\S+/, "html")} />
        </div>
      )}
    </div>
  );
}
