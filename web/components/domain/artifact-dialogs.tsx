"use client";

import { Dialog, DropdownMenu } from "radix-ui";
import { useRouter } from "next/navigation";
import { useRef, useState, type ReactNode } from "react";
import type { Artifact, ArtifactType } from "@/lib/api/client";
import { useAttachArtifact, useBootstrap, useCreateArtifact, useNewVersion } from "@/lib/api/hooks";
import { cx } from "@/lib/cx";
import { formatBytes } from "@/lib/files";
import { blankWireframe, isSvgFile } from "@/lib/mockup";
import { Button, IconButton } from "@/components/ui/button";
import { Segmented } from "@/components/ui/controls";
import { Icon } from "@/components/ui/icon";
import { Field, TextArea, TextInput } from "@/components/ui/primitives";
import { DocTypeTag } from "./labels";
import { floatPanel } from "./pickers";

const TYPES: Array<{ value: ArtifactType; label: string }> = [
  { value: "prd", label: "PRD" },
  { value: "spec", label: "Spec" },
  { value: "adr", label: "ADR" },
  { value: "report", label: "Report" },
  { value: "design", label: "Design" },
  { value: "mockup", label: "Mockup (.svg)" },
];

/** Best guess from a filename; people can change it before uploading. */
export function guessType(name: string, mime: string): ArtifactType {
  const n = name.toLowerCase();
  if (/\bprd\b|requirements/.test(n)) return "prd";
  if (/\badr\b|decision/.test(n)) return "adr";
  if (/report|summary|analysis/.test(n)) return "report";
  if (mime.startsWith("image/") || /\.(fig|sketch|pdf)$/.test(n)) return "design";
  return "spec";
}

export function DialogShell({
  open,
  onOpenChange,
  title,
  children,
  footer,
}: {
  open: boolean;
  onOpenChange: (o: boolean) => void;
  title: ReactNode;
  children: ReactNode;
  footer: ReactNode;
}) {
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-scrim" />
        <Dialog.Content
          aria-describedby={undefined}
          className="fixed left-1/2 top-[8vh] z-50 flex max-h-[84vh] w-[min(720px,calc(100%-32px))] -translate-x-1/2 flex-col overflow-hidden rounded-2xl border border-border-float bg-surface-raised shadow-[var(--shadow-float)] outline-none"
        >
          <div className="flex items-center gap-2 px-[18px] pt-3.5">
            <Dialog.Title className="type-heading">{title}</Dialog.Title>
            <Dialog.Close asChild>
              <IconButton icon="x" label="Close" className="ml-auto" />
            </Dialog.Close>
          </div>
          <div className="flex flex-col gap-4 overflow-y-auto px-[18px] pb-4 pt-3">{children}</div>
          <div className="flex flex-wrap items-center justify-end gap-3 border-t border-track bg-surface-sunken px-[18px] py-3">{footer}</div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

function SelectMenu<T extends string>({
  id,
  value,
  options,
  onChange,
  render,
}: {
  id: string;
  value: T;
  options: Array<{ value: T; label: string }>;
  onChange: (v: T) => void;
  render?: (v: T) => ReactNode;
}) {
  const current = options.find((o) => o.value === value);
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button
          id={id}
          type="button"
          className="flex h-[30px] w-full items-center gap-2 rounded-md border border-border-strong bg-surface px-2.5 text-left text-[13px] hover:bg-hover"
        >
          <span className="flex min-w-0 flex-1 items-center gap-2 truncate">{render ? render(value) : current?.label}</span>
          <Icon name="chev" size={14} className="text-fg-tertiary" />
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align="start" sideOffset={4} className={cx(floatPanel, "z-[60] min-w-[220px]")}>
          {options.map((o) => (
            <DropdownMenu.Item
              key={o.value}
              onSelect={() => onChange(o.value)}
              className="flex h-8 items-center gap-2.5 rounded-md px-2 outline-none data-[highlighted]:bg-hover"
            >
              {render ? render(o.value) : o.label}
            </DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}

function FilePicker({ files, onFiles, multiple, accept, hint }: { files: File[]; onFiles: (f: File[]) => void; multiple?: boolean; accept?: string; hint?: string }) {
  const input = useRef<HTMLInputElement>(null);
  const [over, setOver] = useState(false);
  return (
    <div className="flex flex-col gap-2">
      <button
        type="button"
        onClick={() => input.current?.click()}
        onDragOver={(e) => {
          e.preventDefault();
          setOver(true);
        }}
        onDragLeave={() => setOver(false)}
        onDrop={(e) => {
          e.preventDefault();
          setOver(false);
          const dropped = [...e.dataTransfer.files];
          if (dropped.length) onFiles(multiple ? dropped : dropped.slice(0, 1));
        }}
        className={cx(
          "flex items-center gap-3 rounded-xl border border-dashed px-4 py-4 text-left",
          over ? "border-accent bg-accent-soft" : "border-border-strong hover:bg-hover",
        )}
      >
        <span className="grid size-8 place-items-center rounded-md border border-border-strong bg-selected text-fg-secondary">
          <Icon name="upload" size={15} />
        </span>
        <span className="flex flex-col">
          <span className="font-medium">{multiple ? "Choose files or drop them here" : "Choose a file or drop it here"}</span>
          <span className="text-[12.5px] text-fg-tertiary">{hint ?? "Markdown is outlined and searchable. Other files (PDF, images, exports) up to 25 MB."}</span>
        </span>
      </button>
      <input
        ref={input}
        type="file"
        multiple={multiple}
        accept={accept}
        className="sr-only"
        tabIndex={-1}
        aria-hidden
        onChange={(e) => {
          if (e.target.files?.length) onFiles([...e.target.files]);
          e.target.value = "";
        }}
      />
      {files.length > 0 && (
        <ul className="flex flex-col gap-1">
          {files.map((f, i) => (
            <li key={`${f.name}-${i}`} className="flex items-center gap-2.5 rounded-md bg-surface px-2.5 py-1.5 text-[13px]">
              <Icon name="artifacts" size={14} className="text-fg-tertiary" />
              <span className="flex-1 truncate">{f.name}</span>
              <span className="font-mono text-[11px] text-fg-tertiary">{formatBytes(f.size)}</span>
              <IconButton icon="x" label={`Remove ${f.name}`} size={22} onClick={() => onFiles(files.filter((_, j) => j !== i))} />
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

/** Upload files or write a markdown artifact. Several files make several artifacts. */
export function UploadDialog({
  open,
  onOpenChange,
  initialFiles = [],
  defaultProject,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (o: boolean) => void;
  initialFiles?: File[];
  defaultProject?: string;
  onCreated?: (a: Artifact) => void;
}) {
  const boot = useBootstrap();
  const create = useCreateArtifact();
  const [mode, setMode] = useState<"file" | "write">("file");
  const [files, setFiles] = useState<File[]>(initialFiles);
  const [body, setBody] = useState("");
  const projects = boot.data?.projects.filter(p => !p.archived) ?? [];
  const [projectKey, setProjectKey] = useState(defaultProject ?? projects[0]?.key ?? "");
  const project = projects.find((p) => p.key === projectKey) ?? projects[0];
  const [type, setType] = useState<ArtifactType>(initialFiles[0] ? guessType(initialFiles[0].name, initialFiles[0].type) : "spec");
  const [title, setTitle] = useState("");
  const [epicId, setEpicId] = useState("");
  const [progress, setProgress] = useState<string | null>(null);
  const epics = boot.data?.epics.filter((e) => e.project_id === project?.id) ?? [];

  const ready = project && (mode === "write" ? body.trim().length > 0 : files.length > 0);

  async function submit() {
    if (!ready) return;
    const forms: FormData[] = [];
    const base = () => {
      const f = new FormData();
      f.set("project_key", project.key);
      f.set("type", type);
      if (epicId) f.set("epic_id", epicId);
      return f;
    };
    if (mode === "write") {
      const f = base();
      if (title.trim()) f.set("title", title.trim());
      f.set("body_md", body);
      forms.push(f);
    } else {
      for (const file of files) {
        const f = base();
        if (title.trim() && files.length === 1) f.set("title", title.trim());
        f.set("file", file);
        forms.push(f);
      }
    }
    let last: Artifact | undefined;
    for (const [i, f] of forms.entries()) {
      if (forms.length > 1) setProgress(`Uploading ${i + 1} of ${forms.length}…`);
      try {
        last = await create.mutateAsync(f);
      } catch {
        setProgress(null);
        return; // create.error shows the reason; finished uploads stay.
      }
    }
    setProgress(null);
    onOpenChange(false);
    if (last) onCreated?.(last);
  }

  return (
    <DialogShell
      open={open}
      onOpenChange={onOpenChange}
      title="Add artifact"
      footer={
        <>
          {create.isError && (
            <span role="alert" className="mr-auto text-xs text-pill-blocked-fg">
              {create.error.message}
            </span>
          )}
          {progress && <span className="mr-auto text-xs text-fg-tertiary">{progress}</span>}
          <Dialog.Close asChild>
            <Button variant="ghost">Cancel</Button>
          </Dialog.Close>
          <Button variant="primary" icon="upload" disabled={!ready || create.isPending} onClick={submit}>
            {mode === "write" ? "Create" : files.length > 1 ? `Upload ${files.length}` : "Upload"}
          </Button>
        </>
      }
    >
      <Segmented
        label="Source"
        value={mode}
        onChange={setMode}
        options={[
          { value: "file", label: "Upload" },
          { value: "write", label: "Write markdown" },
        ]}
      />
      {mode === "file" ? (
        <FilePicker
          multiple
          files={files}
          onFiles={(f) => {
            setFiles(f);
            if (f[0]) setType(guessType(f[0].name, f[0].type));
          }}
        />
      ) : (
        <Field label="Markdown" htmlFor="art-body" hint="Headings become the outline agents cite. The first # heading is the title unless you set one.">
          <TextArea id="art-body" mono rows={12} value={body} onChange={(e) => setBody(e.target.value)} placeholder={"# Payout batching — Technical spec\n\n## Goals\n"} />
        </Field>
      )}
      <div className="grid grid-cols-[repeat(auto-fit,minmax(200px,1fr))] gap-3">
        <Field label="Type" htmlFor="art-type">
          <SelectMenu id="art-type" value={type} options={TYPES} onChange={setType} render={(v) => <><DocTypeTag type={v} />{TYPES.find((t) => t.value === v)?.label}</>} />
        </Field>
        <Field label="Project" htmlFor="art-project">
          <SelectMenu
            id="art-project"
            value={project?.key ?? ""}
            options={projects.map((p) => ({ value: p.key, label: p.name }))}
            onChange={(k) => {
              setProjectKey(k);
              setEpicId("");
            }}
            render={(k) => {
              const p = projects.find((x) => x.key === k);
              return p ? (
                <>
                  <span aria-hidden className="size-2.5 rounded-[3px]" style={{ background: p.color }} />
                  {p.name}
                </>
              ) : null;
            }}
          />
        </Field>
        <Field label="Epic" htmlFor="art-epic">
          <SelectMenu
            id="art-epic"
            value={epicId}
            options={[{ value: "", label: "No epic" }, ...epics.map((e) => ({ value: e.id, label: e.name }))]}
            onChange={setEpicId}
          />
        </Field>
      </div>
      {(mode === "write" || files.length <= 1) && (
        <Field label="Title" htmlFor="art-title" hint="Optional. Defaults to the first heading, then the filename.">
          <TextInput id="art-title" value={title} onChange={(e) => setTitle(e.target.value)} placeholder="e.g. Ledger v2 — Product requirements" />
        </Field>
      )}
    </DialogShell>
  );
}

/** New version: edit the markdown in place, or upload a replacement file. */
export function NewVersionDialog({ artifact, open, onOpenChange }: { artifact: Artifact; open: boolean; onOpenChange: (o: boolean) => void }) {
  const isMarkdown = artifact.current_version.content.kind === "markdown";
  const isMockup = artifact.type === "mockup";
  const update = useNewVersion(artifact.id);
  const [mode, setMode] = useState<"write" | "file">(isMarkdown ? "write" : "file");
  const [body, setBody] = useState(artifact.body_md ?? "");
  const [files, setFiles] = useState<File[]>([]);
  const [label, setLabel] = useState("");
  const [note, setNote] = useState("");
  const changed = mode === "write" ? body.trim() !== "" && body !== artifact.body_md : files.length === 1;

  function submit() {
    if (!changed) return;
    const f = new FormData();
    if (label.trim()) f.set("version_label", label.trim());
    if (note.trim()) f.set("note", note.trim());
    if (mode === "write") f.set("body_md", body);
    else f.set("file", files[0]);
    update.mutate(f, { onSuccess: () => onOpenChange(false) });
  }

  return (
    <DialogShell
      open={open}
      onOpenChange={onOpenChange}
      title={`New version of ${artifact.title}`}
      footer={
        <>
          {update.isError && (
            <span role="alert" className="mr-auto text-xs text-pill-blocked-fg">
              {update.error.message}
            </span>
          )}
          <span className="mr-auto text-xs text-fg-tertiary">Current: {artifact.current_version.label}. Agents get the new version on their next read.</span>
          <Dialog.Close asChild>
            <Button variant="ghost">Cancel</Button>
          </Dialog.Close>
          <Button variant="primary" disabled={!changed || update.isPending} onClick={submit} hint={mode === "write" ? "⌘↵" : undefined}>
            Save version
          </Button>
        </>
      }
    >
      {!isMockup && (
        <Segmented
          label="Source"
          value={mode}
          onChange={setMode}
          options={[
            { value: "write", label: "Edit markdown" },
            { value: "file", label: "Upload file" },
          ]}
        />
      )}
      {mode === "write" ? (
        <TextArea
          aria-label="Markdown"
          mono
          rows={16}
          value={body}
          onChange={(e) => setBody(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) submit();
          }}
        />
      ) : (
        <FilePicker
          files={files}
          onFiles={setFiles}
          accept={isMockup ? ".svg,image/svg+xml" : undefined}
          hint={isMockup ? "Upload an .svg to replace the mockup. The new version needs a fresh review." : undefined}
        />
      )}
      <div className="grid grid-cols-[repeat(auto-fit,minmax(200px,1fr))] gap-3">
        <Field label="Version label" htmlFor="ver-label" hint={`Defaults to v${artifact.versions.length + 1}`}>
          <TextInput id="ver-label" mono value={label} onChange={(e) => setLabel(e.target.value)} placeholder={`v${artifact.versions.length + 1}`} />
        </Field>
        <Field label="What changed" htmlFor="ver-note">
          <TextInput id="ver-note" value={note} onChange={(e) => setNote(e.target.value)} placeholder="e.g. Tightened the p95 target" />
        </Field>
      </div>
    </DialogShell>
  );
}

/**
 * New mockup: a blank wireframe template or an uploaded .svg. When launched
 * from a ticket it is attached as context, then the editor opens.
 */
export function NewMockupDialog({
  open,
  onOpenChange,
  defaultProject,
  defaultEpicId,
  ticketKey,
}: {
  open: boolean;
  onOpenChange: (o: boolean) => void;
  defaultProject?: string;
  defaultEpicId?: string | null;
  ticketKey?: string;
}) {
  const router = useRouter();
  const boot = useBootstrap();
  const create = useCreateArtifact();
  const attach = useAttachArtifact(ticketKey ?? "");
  const [mode, setMode] = useState<"template" | "upload">("template");
  const [title, setTitle] = useState("");
  const [files, setFiles] = useState<File[]>([]);
  const [projectKey, setProjectKey] = useState(defaultProject ?? "");
  const [epicId, setEpicId] = useState(defaultEpicId ?? "");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const projects = boot.data?.projects.filter((p) => !p.archived) ?? [];
  const project = projects.find((p) => p.key === projectKey) ?? projects[0];
  const epics = boot.data?.epics.filter((e) => e.project_id === project?.id) ?? [];
  const ready = !!project && (mode === "template" ? title.trim().length > 0 : files.length === 1);

  async function submit() {
    if (!ready || busy) return;
    setBusy(true);
    setError(null);
    const f = new FormData();
    f.set("project_key", project.key);
    f.set("type", "mockup");
    if (epicId) f.set("epic_id", epicId);
    if (title.trim()) f.set("title", title.trim());
    if (mode === "template") f.set("body_md", blankWireframe(title));
    else f.set("file", files[0]);
    try {
      const a = await create.mutateAsync(f);
      if (ticketKey) await attach.mutateAsync(a.id);
      onOpenChange(false);
      router.push(`/artifacts/${a.id}?edit=1`);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Couldn't create the mockup.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <DialogShell
      open={open}
      onOpenChange={onOpenChange}
      title="New mockup"
      footer={
        <>
          {error && (
            <span role="alert" className="mr-auto text-xs text-pill-blocked-fg">
              {error}
            </span>
          )}
          <Dialog.Close asChild>
            <Button variant="ghost">Cancel</Button>
          </Dialog.Close>
          <Button variant="primary" icon="plus" disabled={!ready || busy} onClick={submit}>
            {mode === "template" ? "Create & edit" : "Upload & edit"}
          </Button>
        </>
      }
    >
      <Field label="Title" htmlFor="mockup-title">
        <TextInput id="mockup-title" autoFocus value={title} onChange={(e) => setTitle(e.target.value)} placeholder="e.g. Payout batching — settings screen" />
      </Field>
      <Segmented
        label="Start from"
        value={mode}
        onChange={(m) => {
          setMode(m);
          if (m === "upload" && files[0] && !title.trim()) setTitle(files[0].name.replace(/\.svg$/i, ""));
        }}
        options={[
          { value: "template", label: "Blank wireframe" },
          { value: "upload", label: "Upload .svg" },
        ]}
      />
      {mode === "template" ? (
        <p className="text-[12.5px] text-fg-tertiary">A 1280×800 wireframe with a header, sidebar and content blocks. You&apos;ll edit the SVG source next.</p>
      ) : (
        <FilePicker
          files={files}
          accept=".svg,image/svg+xml"
          hint="One .svg file. Scripts, event handlers and external links are rejected."
          onFiles={(f) => {
            const svg = f.filter((x) => isSvgFile(x.name, x.type));
            setFiles(svg.slice(0, 1));
            if (svg[0] && !title.trim()) setTitle(svg[0].name.replace(/\.svg$/i, ""));
          }}
        />
      )}
      <div className="grid grid-cols-[repeat(auto-fit,minmax(200px,1fr))] gap-3">
        <Field label="Project" htmlFor="mockup-project">
          <SelectMenu
            id="mockup-project"
            value={project?.key ?? ""}
            options={projects.map((p) => ({ value: p.key, label: p.name }))}
            onChange={(k) => {
              setProjectKey(k);
              setEpicId("");
            }}
          />
        </Field>
        <Field label="Epic" htmlFor="mockup-epic">
          <SelectMenu
            id="mockup-epic"
            value={epicId}
            options={[{ value: "", label: "No epic" }, ...epics.map((e) => ({ value: e.id, label: e.name }))]}
            onChange={setEpicId}
          />
        </Field>
      </div>
    </DialogShell>
  );
}
