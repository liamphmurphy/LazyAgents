"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { EditorLayout } from "@/components/shell/editor-layout";

export function EditorLayoutDemo() {
  const [busy, setBusy] = useState(false);
  return (
    <EditorLayout
      app="demo"
      header={{
        back: { href: "/", label: "Home" },
        title: "Onboarding flow",
        saveState: "Saved",
        actions: [<Button key="share">Share</Button>, <Button key="export" variant="primary">Export</Button>],
      }}
      left={<p className="p-4 text-[13px] text-fg-secondary">Layers</p>}
      main={
        <div className="grid flex-1 place-items-center gap-3 p-8 text-fg-secondary">
          <label className="flex items-center gap-2 text-[13px]">
            <input type="checkbox" checked={busy} onChange={(e) => setBusy(e.target.checked)} />
            Persona busy
          </label>
        </div>
      }
      dock={[
        {
          id: "chat",
          label: "Chat",
          icon: "msg",
          busy,
          unread: true,
          content: (
            <div className="flex flex-1 flex-col justify-end p-3">
              <textarea aria-label="Message" placeholder="Ask the designer…" className="h-12 rounded-lg border border-border bg-surface p-2 text-[13px]" />
            </div>
          ),
        },
        { id: "comments", label: "Comments", icon: "at", content: <p className="p-3 text-[13px]">No comments</p> },
        { id: "inspect", label: "Inspect", icon: "sliders", content: <p className="p-3 text-[13px]">Nothing selected</p> },
      ]}
    />
  );
}
