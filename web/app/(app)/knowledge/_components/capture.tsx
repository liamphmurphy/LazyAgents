"use client";

import { useState } from "react";
import { useCapture } from "@/lib/api/knowledge-hooks";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { Kbd } from "@/components/ui/primitives";
import { Modal } from "./kit";

/** ⌥⌘N: a fleeting note into the Inbox, without leaving what you're doing. */
export function CaptureDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (o: boolean) => void }) {
  return (
    <Modal open={open} onOpenChange={onOpenChange} title="Quick capture" className="top-[30vh]">
      {/* Closing unmounts the form, so each capture starts empty. */}
      <CaptureForm onDone={() => onOpenChange(false)} />
    </Modal>
  );
}

function CaptureForm({ onDone }: { onDone: () => void }) {
  const [text, setText] = useState("");
  const capture = useCapture();
  const save = () => {
    const t = text.trim();
    if (!t) return;
    const [first, ...rest] = t.split("\n");
    capture.mutate({ title: first.slice(0, 120), body_md: rest.length ? rest.join("\n").trim() : first.length > 120 ? t : "" }, { onSuccess: onDone });
  };
  return (
    <>
      <div className="flex items-center gap-2 px-5 pt-1 text-12 text-fg-tertiary">
        <Icon name="inbox" size={13} className="text-[#3FC1B0]" />
        Goes to Inbox. File it later, or let a persona file it for you.
      </div>
      <div className="p-4">
        <textarea
          autoFocus
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && (e.metaKey || e.ctrlKey || !e.shiftKey)) {
              e.preventDefault();
              save();
            }
          }}
          rows={3}
          placeholder="A thought, a question, a [[link]]…"
          aria-label="Note"
          className="w-full resize-none rounded-lg border border-border-subtle bg-surface-sunken px-3 py-2.5 text-14 text-fg outline-none placeholder:text-fg-tertiary focus:border-accent"
        />
        {capture.error && <p className="mt-2 text-12 text-state-blocked">{capture.error.message}</p>}
        <div className="mt-3 flex items-center gap-3 text-11 text-fg-tertiary">
          <span>
            <Kbd>↵</Kbd> capture · <Kbd>⇧↵</Kbd> new line
          </span>
          <span className="flex-1" />
          <Button variant="primary" onClick={save} disabled={!text.trim() || capture.isPending}>
            {capture.isPending ? "Capturing…" : "Capture"}
          </Button>
        </div>
      </div>
    </>
  );
}
