"use client";

import { useRef, useState, type DragEvent } from "react";
import { chatAttachmentUrl, postForm, type Schemas } from "@/lib/api/client";
import { ATTACH_ACCEPT, attachmentBadge, attachmentProblem, MAX_ATTACHMENTS } from "@/lib/attachments";
import { formatBytes } from "@/lib/files";
import { cx } from "@/lib/cx";
import { Icon } from "@/components/ui/icon";

type Attachment = Schemas["ChatAttachment"];
type Item = { key: number; name: string; size: number; status: "uploading" | "done" | "error"; id?: string; error?: string };

const chipClass = "inline-flex h-6 max-w-full items-center gap-1.5 rounded-md border px-2 text-xs";

function Badge({ name }: { name: string }) {
  return <span className="flex-none rounded bg-selected px-1 font-mono text-[10px] font-semibold text-fg-secondary">{attachmentBadge(name)}</span>;
}

/**
 * Files for the next message: each one uploads as soon as it's chosen or dropped
 * and becomes a chip. `ids` are the finished uploads; `pending` blocks sending.
 */
export function useAttachments() {
  const [items, setItems] = useState<Item[]>([]);
  const next = useRef(0);
  const count = useRef(0);
  const patch = (key: number, p: Partial<Item>) => setItems((xs) => xs.map((x) => (x.key === key ? { ...x, ...p } : x)));

  function add(files: File[]) {
    for (const f of files) {
      const key = next.current++;
      const problem = attachmentProblem(f) ?? (count.current >= MAX_ATTACHMENTS ? `max ${MAX_ATTACHMENTS} per message` : undefined);
      if (problem) {
        setItems((xs) => [...xs, { key, name: f.name, size: f.size, status: "error", error: problem }]);
        continue;
      }
      count.current++;
      setItems((xs) => [...xs, { key, name: f.name, size: f.size, status: "uploading" }]);
      const form = new FormData();
      form.append("file", f);
      postForm<Attachment>("/chat-attachments", form).then(
        (a) => patch(key, { status: "done", id: a.id }),
        (e: unknown) => {
          count.current--;
          patch(key, { status: "error", error: e instanceof Error ? e.message : "upload failed" });
        },
      );
    }
  }
  function remove(key: number) {
    if (items.find((x) => x.key === key)?.status !== "error") count.current--;
    setItems((xs) => xs.filter((x) => x.key !== key));
  }
  return {
    items,
    add,
    remove,
    pending: items.some((x) => x.status === "uploading"),
    ids: items.flatMap((x) => (x.status === "done" && x.id ? [x.id] : [])),
    clear: () => {
      count.current = 0;
      setItems([]);
    },
  };
}

export type Attachments = ReturnType<typeof useAttachments>;

/** The paperclip: opens the file picker. */
export function AttachButton({ onFiles, disabled }: { onFiles: (f: File[]) => void; disabled?: boolean }) {
  const input = useRef<HTMLInputElement>(null);
  return (
    <>
      <input
        ref={input}
        type="file"
        multiple
        hidden
        accept={ATTACH_ACCEPT}
        data-testid="attach-input"
        onChange={(e) => {
          onFiles([...(e.target.files ?? [])]);
          e.target.value = "";
        }}
      />
      <button
        type="button"
        aria-label="Attach or drop files"
        title="Attach or drop files"
        disabled={disabled}
        onClick={() => input.current?.click()}
        className="inline-flex h-8 items-center gap-1.5 rounded-md px-2 text-[12.5px] text-fg-secondary hover:bg-hover disabled:opacity-50"
      >
        <svg aria-hidden viewBox="0 0 16 16" width={16} height={16} fill="none" stroke="currentColor" strokeWidth={1.5} strokeLinecap="round" strokeLinejoin="round">
          <path d="m13 7.5-4.8 4.8a3 3 0 0 1-4.2-4.2l5-5a2 2 0 0 1 2.8 2.8L6.8 10.9a1 1 0 0 1-1.4-1.4L9.7 5.2" />
        </svg>
        Attach
      </button>
    </>
  );
}

/** Chips above the composer: uploading, ready, or rejected (red, with why). */
export function AttachmentChips({ attachments }: { attachments: Attachments }) {
  if (attachments.items.length === 0) return null;
  return (
    <ul aria-label="Attachments" className="flex flex-wrap gap-1.5" data-testid="attachment-chips">
      {attachments.items.map((x) => (
        <li
          key={x.key}
          data-testid="attachment-chip"
          data-status={x.status}
          className={cx(
            chipClass,
            "relative overflow-hidden",
            x.status === "error" ? "border-pill-blocked-fg/40 bg-surface text-pill-blocked-fg" : "border-border bg-surface text-fg-secondary",
          )}
        >
          {x.status === "error" ? (
            <span role="alert" className="truncate">
              {x.name} · {x.error}
            </span>
          ) : (
            <>
              <Badge name={x.name} />
              <span className="truncate text-fg">{x.name}</span>
              <span className="flex-none text-fg-tertiary">{x.status === "uploading" ? "Uploading…" : formatBytes(x.size)}</span>
            </>
          )}
          <button type="button" aria-label={`Remove ${x.name}`} onClick={() => attachments.remove(x.key)} className="flex-none text-fg-tertiary hover:text-fg">
            <Icon name="x" size={12} />
          </button>
          {x.status === "uploading" && <span aria-hidden className="absolute inset-x-0 bottom-0 h-0.5 animate-pulse bg-accent" />}
        </li>
      ))}
    </ul>
  );
}

/** Files on a sent message, each a link to its download. */
export function SentAttachments({ attachments }: { attachments?: Attachment[] }) {
  if (!attachments?.length) return null;
  return (
    <ul aria-label="Attachments" className="flex flex-wrap justify-end gap-1" data-testid="sent-attachments">
      {attachments.map((a) => (
        <li key={a.id} data-testid="sent-attachment">
          <a
            href={chatAttachmentUrl(a.id)}
            target="_blank"
            rel="noopener noreferrer"
            className={cx(chipClass, "border-border bg-surface text-fg-secondary hover:bg-hover")}
          >
            <Badge name={a.filename} />
            <span className="truncate text-fg">{a.filename}</span>
            <span className="flex-none text-fg-tertiary">{formatBytes(a.size_bytes)}</span>
          </a>
        </li>
      ))}
    </ul>
  );
}

/**
 * Drag and drop over an area: spread `props` on its (relative) container and
 * render <DropOverlay over={over} /> inside it. Enter/leave are counted so the
 * overlay doesn't flicker as the pointer crosses child elements.
 */
export function useFileDrop(onFiles: (f: File[]) => void, enabled = true) {
  const [over, setOver] = useState(false);
  const depth = useRef(0);
  const files = (e: DragEvent) => enabled && e.dataTransfer.types.includes("Files");
  const props = {
    onDragEnter: (e: DragEvent) => {
      if (!files(e)) return;
      e.preventDefault();
      depth.current++;
      setOver(true);
    },
    onDragOver: (e: DragEvent) => {
      if (files(e)) e.preventDefault();
    },
    onDragLeave: (e: DragEvent) => {
      if (!files(e)) return;
      depth.current = Math.max(0, depth.current - 1);
      if (depth.current === 0) setOver(false);
    },
    onDrop: (e: DragEvent) => {
      if (!files(e)) return;
      e.preventDefault();
      depth.current = 0;
      setOver(false);
      onFiles([...e.dataTransfer.files]);
    },
  };
  return { over: over && enabled, props };
}

export function DropOverlay({ over }: { over: boolean }) {
  if (!over) return null;
  return (
    <div
      data-testid="drop-overlay"
      className="pointer-events-none absolute inset-0 z-20 grid place-items-center bg-bg/80 p-4 backdrop-blur-[1px]"
    >
      <div className="flex max-w-[420px] flex-col items-center gap-1 rounded-xl border-2 border-dashed border-accent bg-accent/10 px-10 py-8 text-center">
        <p className="text-[15px] font-semibold text-fg">Drop files to attach</p>
        <p className="text-[12.5px] text-fg-secondary">PDF, images, text, Markdown, CSV · up to 25 MB each</p>
        <p className="text-xs text-fg-tertiary">Files become chips in the composer before you send</p>
      </div>
    </div>
  );
}
