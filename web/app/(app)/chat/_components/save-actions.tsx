"use client";

import Link from "next/link";
import { useId, useState } from "react";
import { useCreateArtifact, useCreateWikiPage } from "@/lib/api/hooks";
import { defaultSavePath, withChatBacklink } from "@/lib/chat-context";
import { wikiHref } from "@/lib/wikilinks";
import { Button } from "@/components/ui/button";
import { Field, TextInput } from "@/components/ui/primitives";
import { DialogShell } from "@/components/domain/artifact-dialogs";

const errorText = (e: unknown) => (e instanceof Error ? e.message : "Something went wrong. Please try again.");

type Props = { chat: { id: string; title: string; project?: string }; body: string };

function SaveDialog({
  title,
  label,
  fieldLabel,
  initial,
  pending,
  error,
  done,
  onSave,
  onClose,
  open,
}: {
  title: string;
  label: string;
  fieldLabel: string;
  initial: string;
  pending: boolean;
  error: unknown;
  done?: { href: string; text: string };
  onSave: (value: string) => void;
  onClose: () => void;
  open: boolean;
}) {
  const [value, setValue] = useState(initial);
  const id = useId();
  return (
    <DialogShell
      open={open}
      onOpenChange={(o) => !o && onClose()}
      title={title}
      footer={
        done ? (
          <>
            <Link href={done.href} className="mr-auto text-[13px] text-accent hover:underline">{done.text}</Link>
            <Button variant="outline" onClick={onClose}>Done</Button>
          </>
        ) : (
          <>
            <Button variant="outline" onClick={onClose}>Cancel</Button>
            <Button variant="primary" disabled={pending || !value.trim()} onClick={() => onSave(value.trim())}>
              {pending ? "Saving…" : label}
            </Button>
          </>
        )
      }
    >
      {!done && (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (!pending && value.trim()) onSave(value.trim());
          }}
        >
          <Field label={fieldLabel} htmlFor={id}>
            <TextInput id={id} autoFocus value={value} onChange={(e) => setValue(e.target.value)} />
          </Field>
        </form>
      )}
      {done && <p role="status" className="text-[13px]">Saved.</p>}
      {error != null && !done && (
        <p role="alert" className="text-[12.5px] text-pill-blocked-fg">{errorText(error)}</p>
      )}
    </DialogShell>
  );
}

/** Save an assistant reply as a wiki page or a report, with a link back to the chat. */
export function SaveActions({ chat, body }: Props) {
  const [mode, setMode] = useState<"wiki" | "artifact" | null>(null);
  const page = useCreateWikiPage();
  const artifact = useCreateArtifact();
  const md = withChatBacklink(body, chat.id, chat.title);
  const close = () => {
    setMode(null);
    page.reset();
    artifact.reset();
  };
  const savedPage = page.data;
  const savedArtifact = artifact.data;

  return (
    <div className="flex flex-wrap items-center gap-1.5 text-xs">
      <Button variant="ghost" icon="wiki" onClick={() => setMode("wiki")}>Save to wiki</Button>
      <Button variant="ghost" icon="artifacts" onClick={() => setMode("artifact")}>Save as artifact</Button>
      {mode === "wiki" && (
        <SaveDialog
          open
          title="Save to wiki"
          label="Save page"
          fieldLabel="Page path"
          initial={defaultSavePath(chat.title)}
          pending={page.isPending}
          error={page.error}
          done={savedPage && { href: wikiHref(savedPage.path ?? ""), text: `Open ${savedPage.path ?? "page"}` }}
          onSave={(path) => page.mutate({ path, body_md: md, project_key: chat.project })}
          onClose={close}
        />
      )}
      {mode === "artifact" && (
        <SaveDialog
          open
          title="Save as artifact"
          label="Save report"
          fieldLabel="Title"
          initial={chat.title}
          pending={artifact.isPending}
          error={artifact.error}
          done={savedArtifact && { href: `/artifacts/${savedArtifact.id}`, text: `Open ${savedArtifact.title}` }}
          onSave={(title) => {
            const f = new FormData();
            if (chat.project) f.set("project_key", chat.project);
            f.set("type", "report");
            f.set("title", title);
            f.set("body_md", md);
            artifact.mutate(f);
          }}
          onClose={close}
        />
      )}
    </div>
  );
}
