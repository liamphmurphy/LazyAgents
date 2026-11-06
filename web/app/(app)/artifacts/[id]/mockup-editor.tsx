"use client";

import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import type { Artifact } from "@/lib/api/client";
import { ApiError } from "@/lib/api/client";
import { useNewVersion, useReviewMockup } from "@/lib/api/hooks";
import { insertIndent, svgError, type Zoom } from "@/lib/mockup";
import { Button } from "@/components/ui/button";
import { Field, TextInput } from "@/components/ui/primitives";
import { MockupImage } from "@/components/domain/mockup";
import { ZoomControl } from "./mockup-parts";

/**
 * Split-view SVG editor: source on the left, live preview on the right.
 * Saving always creates a new version (pending review); "Save & approve"
 * also approves it.
 */
export function MockupEditor({ a, onDone }: { a: Artifact; onDone: () => void }) {
  const newVersion = useNewVersion(a.id);
  const review = useReviewMockup(a.id);
  const [baseline, setBaseline] = useState(a.body_md ?? "");
  const [source, setSource] = useState(baseline);
  const [preview, setPreview] = useState(baseline);
  const [note, setNote] = useState("");
  const [zoom, setZoom] = useState<Zoom>("fit");
  const [error, setError] = useState<string | null>(null);
  const area = useRef<HTMLTextAreaElement>(null);
  const caret = useRef<number | null>(null);

  const problem = useMemo(() => svgError(source), [source]);
  const dirty = source !== baseline;
  const busy = newVersion.isPending || review.isPending;
  const canSave = !problem && dirty && !busy;

  // Debounced preview; the last well-formed source stays on screen while the markup is broken.
  useEffect(() => {
    if (problem) return;
    const id = setTimeout(() => setPreview(source), 250);
    return () => clearTimeout(id);
  }, [source, problem]);

  useLayoutEffect(() => {
    if (caret.current != null && area.current) {
      area.current.setSelectionRange(caret.current, caret.current);
      caret.current = null;
    }
  });

  useEffect(() => {
    if (!dirty) return;
    const warn = (e: BeforeUnloadEvent) => e.preventDefault();
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty]);

  async function save(approve: boolean) {
    if (!canSave) return;
    setError(null);
    const form = new FormData();
    form.set("body_md", source);
    if (note.trim()) form.set("note", note.trim());
    let saved: Artifact;
    try {
      saved = await newVersion.mutateAsync(form);
    } catch (e) {
      setError(e instanceof ApiError || e instanceof Error ? e.message : "Couldn't save the mockup.");
      return;
    }
    setBaseline(source);
    if (approve) {
      try {
        await review.mutateAsync({ versionId: saved.current_version.id, decision: "approved" });
      } catch (e) {
        setError(`Saved as ${saved.current_version.label}, but approving failed: ${e instanceof Error ? e.message : "unknown error"}`);
        return;
      }
    }
    onDone();
  }

  function cancel() {
    if (dirty && !window.confirm("Discard your unsaved changes to this mockup?")) return;
    onDone();
  }

  return (
    <div className="flex flex-1 flex-col">
      <div className="flex flex-wrap items-end gap-3 border-b border-border-subtle px-6 py-3">
        <div className="min-w-[220px] flex-1">
          <Field label="What changed" htmlFor="mockup-note">
            <TextInput id="mockup-note" value={note} onChange={(e) => setNote(e.target.value)} placeholder="Optional note for this version" />
          </Field>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Button variant="ghost" onClick={cancel}>
            Cancel
          </Button>
          <Button variant="secondary" disabled={!canSave} onClick={() => save(false)} hint="⌘S">
            Save version
          </Button>
          <Button variant="approve" icon="check" disabled={!canSave} onClick={() => save(true)}>
            Save &amp; approve
          </Button>
        </div>
      </div>
      {(error || problem) && (
        <p role="alert" className="border-b border-border-subtle px-6 py-2 text-xs text-pill-blocked-fg">
          {error ?? problem}
        </p>
      )}
      <div className="grid min-h-[480px] flex-1 grid-cols-1 gap-4 p-4 lg:grid-cols-2 lg:p-6">
        <div className="flex min-h-[320px] flex-col gap-2">
          <label htmlFor="mockup-source" className="text-[12.5px] font-medium text-fg-2">
            SVG source <span className="font-normal text-fg-tertiary">· Tab indents, ⌘S saves</span>
          </label>
          <textarea
            id="mockup-source"
            ref={area}
            value={source}
            spellCheck={false}
            autoCapitalize="off"
            autoCorrect="off"
            aria-invalid={!!problem}
            onChange={(e) => {
              setSource(e.target.value);
              setError(null);
            }}
            onKeyDown={(e) => {
              if (e.key === "Tab" && !e.shiftKey && !e.metaKey && !e.ctrlKey && !e.altKey) {
                e.preventDefault();
                const t = e.currentTarget;
                const r = insertIndent(t.value, t.selectionStart, t.selectionEnd);
                caret.current = r.caret;
                setSource(r.value);
              }
              if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "s") {
                e.preventDefault();
                void save(false);
              }
            }}
            className="min-h-[320px] w-full flex-1 resize-none rounded-md border border-border-strong bg-surface-sunken p-3 font-mono text-[12.5px] leading-relaxed text-fg [tab-size:2] focus:border-accent focus:outline-none aria-[invalid=true]:border-pill-blocked-fg"
          />
        </div>
        <div className="flex min-h-[320px] min-w-0 flex-col gap-2">
          <div className="flex items-center justify-between gap-2">
            <h2 className="text-[12.5px] font-medium text-fg-2">Preview</h2>
            <ZoomControl value={zoom} onChange={setZoom} />
          </div>
          <MockupImage source={preview} alt={`Preview of ${a.title}`} zoom={zoom} className="min-h-[320px] flex-1" />
        </div>
      </div>
    </div>
  );
}
