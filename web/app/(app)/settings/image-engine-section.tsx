"use client";

import { useImageEngine, useSetImageEngine } from "@/lib/api/image-engine-hooks";
import { cx } from "@/lib/cx";
import { engineChoices } from "@/lib/image-engine";
import { LoadingLines } from "@/components/ui/loading";

/** Settings: which engine makes images. A persona or a workflow step can choose its own. */
export function ImageEngineSection() {
  const engine = useImageEngine();
  const save = useSetImageEngine();
  if (engine.isPending) return <LoadingLines />;
  if (engine.error) return <p role="alert" className="text-14 text-pill-blocked-fg">{engine.error.message}</p>;
  const choices = engineChoices(engine.data.options);
  return (
    <div className="flex flex-col gap-2">
      <div role="radiogroup" aria-label="Image engine" className="flex flex-col gap-1.5">
        {choices.map((c) => {
          const on = engine.data.engine === c.value;
          return (
            <button key={c.value} type="button" role="radio" aria-checked={on} disabled={save.isPending || c.disabled} onClick={() => !on && save.mutate(c.value)}
              className={cx("flex items-start gap-2.5 rounded-[8px] border px-3 py-2 text-left transition-colors", on ? "border-accent bg-accent-soft" : "border-border hover:bg-hover")}>
              <span aria-hidden className={cx("mt-[3px] size-[12px] flex-none rounded-full border", on ? "border-accent bg-accent" : "border-border-strong")} />
              <span className="flex flex-col">
                <span className="text-13 font-medium">{c.label}</span>
                <span className="text-12 text-fg-tertiary">{c.detail}</span>
              </span>
            </button>
          );
        })}
      </div>
      <p className="text-12 text-fg-tertiary">A persona can choose its own engine in Manage personas, and a workflow image step in the workflow editor.</p>
      {save.error && <p role="alert" className="text-12 text-pill-blocked-fg">{save.error.message}</p>}
    </div>
  );
}
