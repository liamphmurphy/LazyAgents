"use client";

import { useEffect, useRef, useState, type ReactNode } from "react";
import { cx } from "@/lib/cx";
import { Icon, type IconName } from "@/components/ui/icon";

function ZoomDialog({ label, onClose, children }: { label: string; onClose: () => void; children: ReactNode }) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    ref.current?.showModal();
  }, []);
  return (
    <dialog
      ref={ref}
      aria-label={label}
      onClose={onClose}
      onClick={(e) => {
        if (e.target === ref.current) ref.current?.close();
      }}
      className="m-auto max-h-[92vh] w-[min(92vw,1100px)] overflow-auto rounded-xl border border-border-float bg-surface p-3 text-fg backdrop:bg-scrim"
    >
      <div className="mb-2 flex justify-end">
        <button type="button" aria-label="Close" className="flex items-center rounded p-1 text-fg-tertiary hover:text-fg" onClick={() => ref.current?.close()}>
          <Icon name="x" size={14} />
        </button>
      </div>
      {children}
    </dialog>
  );
}

/** The chrome every visual shares, in Chat and Learn: header, views, actions, caption and zoom. */
export function VisualFrame<V extends string>({ testId, icon, label, views, view, onView, actions, caption, zoom, children }: {
  testId: string;
  icon: IconName;
  label: string;
  views?: { value: V; label: string }[];
  view?: V;
  onView?: (v: V) => void;
  actions?: ReactNode;
  caption?: ReactNode;
  /** What the zoom button shows; without it there's no button. */
  zoom?: ReactNode;
  children: ReactNode;
}) {
  const [zoomed, setZoomed] = useState(false);
  return (
    <figure className="m-0 overflow-hidden break-inside-avoid rounded-lg border border-border bg-surface-sunken" data-testid={testId}>
      <div className="flex items-center justify-between gap-2 border-b border-border-row px-3 py-1 text-12 text-fg-tertiary">
        <span className="flex items-center gap-1.5">
          <Icon name={icon} size={12} />
          <span>{label}</span>
        </span>
        <span className="flex items-center gap-1 print:hidden">
          {views && (
            <span role="tablist" aria-label={`${label} view`} className="flex items-center gap-0.5">
              {views.map((v) => (
                <button key={v.value} type="button" role="tab" aria-selected={view === v.value} className={cx("rounded px-1.5", view === v.value ? "bg-surface text-fg" : "hover:text-fg")} onClick={() => onView?.(v.value)}>
                  {v.label}
                </button>
              ))}
            </span>
          )}
          {actions}
          {zoom && (
            <button type="button" aria-label="Zoom" title="Zoom" className="flex items-center rounded px-1 hover:text-fg" onClick={() => setZoomed(true)}>
              <Icon name="expand" size={12} />
            </button>
          )}
        </span>
      </div>
      {children}
      {caption && <figcaption className="border-t border-border-row px-3 py-2 text-13 text-fg-secondary">{caption}</figcaption>}
      {zoomed && zoom && (
        <ZoomDialog label={label} onClose={() => setZoomed(false)}>
          {zoom}
        </ZoomDialog>
      )}
    </figure>
  );
}
