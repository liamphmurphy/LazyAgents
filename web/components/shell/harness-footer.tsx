"use client";

import Link from "next/link";
import { Popover } from "radix-ui";
import { useId, useRef, useState } from "react";
import { usePersonaDock } from "@/lib/api/hooks";
import { ANTIGRAVITY, checkedAgo, harnessWorking, knownHarnesses, sessionPercent, usageSub, usageTone, type HarnessActivity } from "@/lib/harness-activity";
import { Icon } from "@/components/ui/icon";
import { HarnessCharacter } from "./harness-character";

function UsageRow({ h, all }: { h: HarnessActivity; all: HarnessActivity[] }) {
  const pct = sessionPercent(h);
  const primary = h.usage_available ? h.windows.find((w) => w.kind === "session") ?? h.windows[0] : undefined;
  const working = harnessWorking(h);
  return (
    <div className="harness-usage-row" data-harness={h.harness} data-tone={usageTone(h)}>
      <div className="harness-usage-heading">
        <HarnessCharacter harness={h.harness} working={working} />
        <h3>{h.label}</h3>
        <span className="harness-usage-value">{pct === null ? (h.usage_checking ? "…" : "—") : `${pct}%`}</span>
      </div>
      <div className="harness-usage-track" data-unavailable={pct === null}
        role={pct === null ? undefined : "progressbar"} aria-label={`${h.label} allowance used`}
        aria-valuemin={pct === null ? undefined : 0} aria-valuemax={pct === null ? undefined : 100} aria-valuenow={pct ?? undefined}>
        {pct !== null && <i style={{ width: `${Math.max(0, Math.min(100, pct))}%` }} />}
      </div>
      <div className="harness-usage-meta">
        <span>{primary ? `${primary.label} allowance used` : h.harness === ANTIGRAVITY ? "Allowance not reported" : h.usage_checking ? "Checking usage…" : "Usage unavailable"}</span>
        <span>{h.limited_until ? "Limited" : working ? `${h.running} working` : "Idle"}</span>
      </div>
      <p className="harness-usage-sub">{usageSub(h, all)}</p>
    </div>
  );
}

/** A home for the three harnesses; the drawer exposes readings, not inferred usage. */
export function HarnessFooter({ rail = false }: { rail?: boolean }) {
  const dock = usePersonaDock();
  const harnesses = knownHarnesses(dock.data?.harnesses ?? []);
  const working = harnesses.filter(harnessWorking);
  const running = working.reduce((n, h) => n + h.running, 0);
  const [open, setOpen] = useState(false);
  const anchor = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const id = useId();
  const summary = dock.isPending ? "Loading…" : dock.isError ? "Unavailable" : working.length ? `${working.length} working` : "All idle";

  function toggle(button: HTMLButtonElement) {
    trigger.current = button;
    setOpen((v) => !v);
  }

  return (
    <Popover.Root open={open} onOpenChange={setOpen}>
      <Popover.Anchor asChild>
        <div ref={anchor} role="region" aria-label="Harness activity" className="harness-footer" data-rail={rail}>
          <div className="harness-characters">
            {harnesses.map((h) => (
              <button key={h.harness} type="button" className="harness-button" data-harness={h.harness}
                aria-label={`${h.label}, ${dock.isPending ? "loading" : dock.isError ? "activity unavailable" : h.limited_until ? "limited" : harnessWorking(h) ? "working" : "idle"}; show usage`}
                title={`${h.label} · ${dock.isPending ? "Loading…" : dock.isError ? "Activity unavailable" : h.limited_until ? "Limited" : harnessWorking(h) ? `${h.running} working` : "Idle"}`}
                aria-haspopup="dialog" aria-expanded={open} aria-controls={id} onClick={(e) => toggle(e.currentTarget)}>
                <HarnessCharacter harness={h.harness} working={harnessWorking(h)} />
                <span className="harness-name">{h.harness === ANTIGRAVITY ? "AG" : h.label}</span>
                {harnessWorking(h) && <i aria-hidden className="harness-working-dot" />}
              </button>
            ))}
          </div>
          <button type="button" className="harness-usage-trigger" aria-label={`Harness usage: ${summary}`}
            aria-haspopup="dialog" aria-expanded={open} aria-controls={id} onClick={(e) => toggle(e.currentTarget)}>
            <span className="harness-footer-label">Harnesses</span>
            <span className="harness-footer-summary">{summary}</span>
            <Icon name="chevr" size={12} />
          </button>
        </div>
      </Popover.Anchor>
      <Popover.Portal>
        <Popover.Content id={id} side="right" align="end" sideOffset={0} collisionPadding={12} aria-label="Harness usage"
          className="harness-drawer"
          onInteractOutside={(e) => { if (e.target instanceof Node && anchor.current?.contains(e.target)) e.preventDefault(); }}
          onCloseAutoFocus={(e) => { e.preventDefault(); if (trigger.current?.isConnected) trigger.current.focus(); }}>
          <div className="harness-drawer-heading">
            <div><h2>Harnesses</h2><p>{dock.isPending ? "Loading activity…" : dock.isError ? "Activity unavailable" : `${running} ${running === 1 ? "agent" : "agents"} working`}</p></div>
            <Popover.Close asChild><button type="button" aria-label="Close usage drawer"><Icon name="x" size={16} /></button></Popover.Close>
          </div>
          <div className="harness-drawer-scroll">
            {dock.isError && <p role="alert" className="harness-load-error">Couldn’t load harness activity. <button type="button" onClick={() => void dock.refetch()}>Retry</button></p>}
            {harnesses.map((h) => <UsageRow key={h.harness} h={h} all={harnesses} />)}
          </div>
          <div className="harness-drawer-footer">
            <span>{checkedAgo(harnesses)}</span>
            <Link href="/settings/runner" onClick={() => setOpen(false)}>Usage settings →</Link>
          </div>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}
