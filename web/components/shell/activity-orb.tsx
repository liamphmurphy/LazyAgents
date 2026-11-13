"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useCallback, useEffect, useLayoutEffect, useRef, useState, useSyncExternalStore, type CSSProperties, type ReactNode } from "react";
import { PersonaAvatar } from "@/app/(app)/chat/_components/persona";
import type { Schemas } from "@/lib/api/client";
import { useMarkChatRead, usePersonaDock } from "@/lib/api/hooks";
import { ANTIGRAVITY, CLAUDE, CODEX, ORB_HARNESSES, checkedAgo, chatStates, finishes, orbBadge, orbGlyph, orbLabel, orbQuiet, ringTone, sessionPercent, usageSub, workingHarnesses, type HarnessActivity } from "@/lib/activity-orb";
import { dockEntries, viewedChatId, type DockState } from "@/lib/persona-dock";

type Entry = Schemas["PersonaDockEntry"];
type Chat = Schemas["PersonaDockChat"];
type Persona = Entry["persona"];

/*
 * The one floating element: harness activity (Claude's robot, Codex's comets,
 * Antigravity's lift-off), the session-usage rings in each harness's colour
 * (Antigravity's stays a bare track: it has no allowance to read), and a badge for personas that need the person
 * or replied. Clicking it sprouts a panel out to the left; a persona that
 * finishes sends a chip out of it. The motion is a straight port of the
 * approved mockup: keep its keyframes, timings and easings in step with it.
 */

const ORB = 46;
const CHIP_DONE_MS = 4500;
const CHIP_NEEDS_MS = 8000;
const CHIP_LINGER_MS = 2000;
const CHIPS_MAX = 2;

/** Clears bottom bars and floating buttons (bulk bar, jump-to-latest); the compact tab bar is cleared through --tab-bar-h. */
const ORB_BOTTOM = 80;

const still = () => window.matchMedia("(prefers-reduced-motion: reduce)").matches;

function subscribeVisible(fn: () => void) {
  document.addEventListener("visibilitychange", fn);
  return () => document.removeEventListener("visibilitychange", fn);
}

/** Lifts the orb above a composer anchored to the bottom of the screen. Composers grow and mount late, so it re-measures. */
function useOrbBottom(): number {
  const [bottom, setBottom] = useState(ORB_BOTTOM);
  useEffect(() => {
    const measure = () => {
      const r = document.querySelector("[data-dock-avoid]")?.getBoundingClientRect();
      setBottom(r && r.bottom > window.innerHeight - 120 ? Math.max(ORB_BOTTOM, Math.ceil(window.innerHeight - r.top + 12)) : ORB_BOTTOM);
    };
    measure();
    const t = setInterval(measure, 500);
    window.addEventListener("resize", measure);
    return () => {
      clearInterval(t);
      window.removeEventListener("resize", measure);
    };
  }, []);
  return bottom;
}

// ---- Glyphs: the mockup's ring(), robot(), orbit() and idle(), in a 44×44 box.

/**
 * One harness's session ring in its own colour, amber and breathing near the
 * limit (amber alone is too close to Claude's orange), red once limited. It
 * eases to each new reading, and a faint arc sweeps the track while a usage
 * check is in flight.
 */
function Ring({ r, h, color }: { r: number; h: HarnessActivity | undefined; color: string }) {
  const c = 2 * Math.PI * r;
  const pct = sessionPercent(h) ?? 0;
  const tone = ringTone(h);
  const stroke = tone === "limited" ? "var(--state-blocked)" : tone === "warn" ? "var(--state-human)" : color;
  return (
    <>
      <circle cx="22" cy="22" r={r} fill="none" stroke="var(--orb-track)" strokeWidth="2.4" />
      {h?.usage_checking && (
        <circle className="orb-sweep" cx="22" cy="22" r={r} fill="none" stroke={color} strokeWidth="2.4" strokeLinecap="round" strokeDasharray={`${c * 0.16} ${c}`} />
      )}
      <circle className="orb-ring" data-tone={tone} cx="22" cy="22" r={r} fill="none" strokeWidth="2.4" strokeLinecap="round" transform="rotate(-90 22 22)"
        style={{ stroke, strokeDasharray: `${(c * pct) / 100} ${c}`, opacity: pct > 0 ? 1 : 0 }} />
    </>
  );
}

/** Claude's pixel robot on an 11×8 grid, `u` units per cell, centred in the 44×44 box. Eyes are cut-outs in the orb surface. */
function Robot({ u }: { u: number }) {
  const ox = 22 - 5.5 * u, oy = 22.5 - 4 * u;
  const cell = (x: number, y: number, w: number, h: number, className?: string) => (
    <rect key={`${x}-${y}`} className={className} x={ox + x * u} y={oy + y * u} width={w * u} height={h * u} />
  );
  return (
    <g className="orb-robot" fill="var(--orb-claude)" style={{ "--u": u } as CSSProperties}>
      <g className="orb-robot-body">
        {cell(1, 0, 9, 6)}
        {cell(0, 2, 1, 2)}
        {cell(10, 2, 1, 2)}
        {cell(3, 1, 1, 2, "orb-robot-eye")}
        {cell(7, 1, 1, 2, "orb-robot-eye")}
      </g>
      {[2, 4, 6, 8].map((x, i) => cell(x, 6, 1, 2, i % 2 ? "orb-robot-leg b" : "orb-robot-leg a"))}
    </g>
  );
}

/** Antigravity's lift-off: a core floating over its shadow while particles rise off it, fading. */
function Lift() {
  return (
    <g className="orb-lift" fill="var(--orb-antigravity)">
      <ellipse className="orb-lift-shadow" cx="22" cy="29.5" rx="5.5" ry="1.5" opacity="0.35" />
      <circle className="orb-lift-core" cx="22" cy="21" r="4" />
      {[[17, 25, 0], [27, 24, 0.6], [22, 26, 1.2]].map(([x, y, delay]) => (
        <circle key={x} className="orb-lift-p" cx={x} cy={y} r="1.1" style={{ animationDelay: `${delay}s` }} />
      ))}
    </g>
  );
}

const COMET: [number, number, number][] = [[0, 2.1, 1], [-28, 1.7, 0.6], [-56, 1.3, 0.3]];

function Orbit({ r }: { r: number }) {
  return (
    <g className="orb-orbit" fill="var(--orb-codex)">
      {[0, 180].flatMap((b) =>
        COMET.map(([d, rr, o]) => {
          const a = ((b + d) * Math.PI) / 180;
          return <circle key={`${b}${d}`} cx={22 + r * Math.cos(a)} cy={22 + r * Math.sin(a)} r={rr} opacity={o} />;
        }),
      )}
    </g>
  );
}

const Idle = () => <circle cx="22" cy="22" r="2.5" fill="var(--orb-idle)" />;

/** One harness's glyph at `scale` around the box's centre, shifted by (dx, dy). */
function Glyph({ id, scale, dx = 0, dy = 0 }: { id: string; scale: number; dx?: number; dy?: number }) {
  return (
    <g transform={`translate(${dx} ${dy}) translate(22 22) scale(${scale}) translate(-22 -22)`}>
      {id === CODEX ? <Orbit r={7.5} /> : id === ANTIGRAVITY ? <Lift /> : <Robot u={2} />}
    </g>
  );
}

/** Where each of several working harnesses' small glyph sits, so none overlaps the innermost ring. */
const MULTI_SPOTS: Record<number, [number, number][]> = { 2: [[-6, 0], [6, 0]], 3: [[0, -5.5], [-6.5, 4.5], [6.5, 4.5]] };

function Centre({ harnesses }: { harnesses: HarnessActivity[] }) {
  const g = orbGlyph(harnesses);
  if (g === "idle") return <Idle />;
  if (g === "multi") {
    const working = workingHarnesses(harnesses);
    const spots = MULTI_SPOTS[working.length] ?? MULTI_SPOTS[3];
    return <>{working.map((id, i) => <Glyph key={id} id={id} scale={0.46} dx={spots[i][0]} dy={spots[i][1]} />)}</>;
  }
  // Three rings leave less room than two did, so a lone glyph is a little smaller.
  return <Glyph id={g === "claude" ? CLAUDE : g === "codex" ? CODEX : ANTIGRAVITY} scale={g === "claude" ? 0.8 : 0.9} />;
}

function Mini({ harness }: { harness: string }) {
  return (
    <svg width="20" height="20" viewBox="0 0 44 44" aria-hidden>
      {harness === CODEX ? <Orbit r={10} /> : harness === ANTIGRAVITY ? <g transform="translate(22 22) scale(1.5) translate(-22 -22)"><Lift /></g> : <Robot u={3.4} />}
    </svg>
  );
}

// ---- Panel rows

function Status({ chat }: { chat: Chat }) {
  if (chat.state === "working") return <span className="orb-ps"><Mini harness={chat.harness} />Working</span>;
  if (chat.state === "needs_you") return <span className="orb-ps h">Needs you</span>;
  return <span className="orb-ps"><span className="orb-dot" />Done{chat.unread ? ` · ${chat.unread} new` : ""}</span>;
}

/** Each harness's ring colour, so a panel row can be matched to its ring. */
const RING_COLOR: Record<string, string> = { [CLAUDE]: "var(--orb-claude)", [CODEX]: "var(--orb-codex)", [ANTIGRAVITY]: "var(--orb-antigravity)" };

function UsageRow({ h, all }: { h: HarnessActivity; all: HarnessActivity[] }) {
  const pct = sessionPercent(h);
  const tone = ringTone(h);
  return (
    <>
      <div className="orb-urow" data-stg>
        <span className="orb-ulabel"><i aria-hidden style={{ background: RING_COLOR[h.harness] }} />{h.label}</span>
        <div className="orb-track"><i className={tone === "limited" ? "red" : tone === "warn" ? "warn" : ""} style={{ width: `${pct ?? 0}%` }} /></div>
        <span className="orb-val">{pct === null ? (h.usage_checking ? "…" : "—") : `${pct}%`}</span>
      </div>
      <div className={`orb-usub${tone === "limited" ? " red" : ""}`} data-stg>{usageSub(h, all)}</div>
    </>
  );
}

// ---- Chips: "Nora finished" sprouting out of the orb's left side.

type ChipData = { key: string; kind: "d" | "h"; persona: Persona; chatId: string; title: string; leaving: boolean };

function Chip({ chip, onOpen, onGone }: { chip: ChipData; onOpen: (c: ChipData) => void; onGone: (key: string) => void }) {
  const ref = useRef<HTMLButtonElement>(null);
  const gone = useRef(false);
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);

  const dismiss = useCallback(() => {
    const c = ref.current;
    if (!c || gone.current) return;
    gone.current = true;
    clearTimeout(timer.current);
    const a = still()
      ? c.animate([{ opacity: 1 }, { opacity: 0 }], { duration: 120 })
      : c.animate([{ clipPath: "inset(0 0 0 0 round 12px)", opacity: 1 }, { clipPath: "inset(0 0 0 100% round 23px)", opacity: 0.6 }], { duration: 320, easing: "cubic-bezier(.4,0,.2,1)" });
    a.onfinish = () => {
      c.style.visibility = "hidden";
      onGone(chip.key);
    };
  }, [chip.key, onGone]);

  const arm = useCallback((ms: number) => {
    clearTimeout(timer.current);
    timer.current = setTimeout(dismiss, ms);
  }, [dismiss]);

  useLayoutEffect(() => {
    const c = ref.current;
    if (!c) return;
    if (still()) c.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 150 });
    else {
      c.animate([{ clipPath: "inset(0 0 0 100% round 23px)", transform: "translateX(8px)" }, { clipPath: "inset(0 0 0 0 round 12px)", transform: "none" }], { duration: 440, delay: 120, easing: "cubic-bezier(.2,.9,.25,1.05)", fill: "backwards" });
      c.querySelectorAll(".orb-chip-k").forEach((k, i) => k.animate([{ opacity: 0, transform: "translateX(10px)" }, { opacity: 1, transform: "none" }], { duration: 240, delay: 300 + i * 60, easing: "ease-out", fill: "backwards" }));
    }
    arm(chip.kind === "h" ? CHIP_NEEDS_MS : CHIP_DONE_MS);
    return () => clearTimeout(timer.current);
    // Mount only: a chip enters once.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (chip.leaving) dismiss();
  }, [chip.leaving, dismiss]);

  return (
    <button ref={ref} type="button" className={`orb-chip ${chip.kind}`} onMouseEnter={() => clearTimeout(timer.current)} onMouseLeave={() => !gone.current && arm(CHIP_LINGER_MS)}
      onClick={() => { onOpen(chip); dismiss(); }}>
      <span className="orb-chip-k flex"><PersonaAvatar persona={chip.persona} size={28} /></span>
      <span className="tx orb-chip-k">
        <span><b>{chip.persona.name}</b> <span className="verb">{chip.kind === "h" ? "needs you" : "finished"}</span></span>
        <small>{chip.title || "Untitled chat"}</small>
      </span>
      <span className="go orb-chip-k">{chip.kind === "h" ? "Review" : "Open"}</span>
    </button>
  );
}

// ---- The orb

/**
 * The persona dock and the chat on screen, which it marks read: opening a chat reads it, and so
 * does a reply arriving while it's on screen. The orb uses it, and so does the compact shell without it.
 */
function useViewedChat() {
  const pathname = usePathname();
  const dock = usePersonaDock();
  const visible = useSyncExternalStore(subscribeVisible, () => document.visibilityState === "visible", () => false);
  const viewed = visible ? viewedChatId(pathname) : null;
  const unreadViewed = dock.data?.personas.flatMap((e) => e.chats).find((c) => c.id === viewed && c.unread > 0);
  const { mutate } = useMarkChatRead();
  const seq = unreadViewed?.last_seq;
  useEffect(() => {
    if (viewed && seq) mutate({ id: viewed, seq });
  }, [viewed, seq, mutate]);
  return { dock, viewed };
}

/** Marks the chat on screen read where the orb doesn't render (the compact shell). */
export function ViewedChatReader() {
  useViewedChat();
  return null;
}

/** Which personas and harnesses are working, what's left of each plan, and who finished, on every screen. */
export function ActivityOrb() {
  const router = useRouter();
  const { dock, viewed } = useViewedChat();
  const bottom = useOrbBottom();
  const entries = dockEntries<Chat, Entry>(dock.data?.personas ?? [], viewed);
  const harnesses: HarnessActivity[] = dock.data?.harnesses ?? [];

  const root = useRef<HTMLDivElement>(null);
  const btn = useRef<HTMLButtonElement>(null);
  const panel = useRef<HTMLDivElement>(null);
  const fx = useRef<HTMLDivElement>(null);
  const anim = useRef<Animation>(undefined);
  const [open, setOpen] = useState(false);
  const openRef = useRef(false);
  const [chips, setChips] = useState<ChipData[]>([]);
  const [live, setLive] = useState("");

  // ---- Sprout: the orb's circle stretches left into a pill, then up into the card.
  const frames = useCallback((r: number): Keyframe[] => {
    const p = panel.current!;
    const W = p.offsetWidth, H = p.offsetHeight;
    return [
      { clipPath: `inset(${H - ORB}px 0px 0px ${W - ORB}px round ${ORB / 2}px)` },
      { clipPath: `inset(${H - ORB}px 0px 0px 0px round ${ORB / 2}px)`, offset: 0.42 },
      { clipPath: `inset(0px 0px 0px 0px round ${r}px)` },
    ];
  }, []);

  const show = useCallback(() => {
    const p = panel.current;
    if (!p) return;
    openRef.current = true;
    setOpen(true);
    setChips((cs) => cs.map((c) => ({ ...c, leaving: true })));
    p.style.visibility = "visible";
    anim.current?.cancel();
    if (still()) {
      anim.current = p.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 120, fill: "forwards" });
      return;
    }
    anim.current = p.animate(frames(14), { duration: 540, easing: "cubic-bezier(.2,.8,.2,1)", fill: "forwards" });
    p.querySelectorAll("[data-stg]").forEach((el, i) => el.animate([{ opacity: 0, transform: "translateX(14px)" }, { opacity: 1, transform: "none" }], { duration: 280, delay: 220 + i * 45, easing: "ease-out", fill: "backwards" }));
  }, [frames]);

  const hide = useCallback(() => {
    const p = panel.current;
    if (!p || !openRef.current) return;
    openRef.current = false;
    setOpen(false);
    anim.current?.cancel();
    anim.current = still()
      ? p.animate([{ opacity: 1 }, { opacity: 0 }], { duration: 100, fill: "forwards" })
      : p.animate(frames(14).reverse().map((f, i) => (i === 1 ? { ...f, offset: 0.58 } : f)), { duration: 380, easing: "cubic-bezier(.4,0,.2,1)", fill: "forwards" });
    anim.current.onfinish = () => {
      if (!openRef.current) p.style.visibility = "hidden";
    };
  }, [frames]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && openRef.current) {
        hide();
        btn.current?.focus();
      }
    };
    const onDown = (e: MouseEvent) => {
      if (!root.current?.contains(e.target as Node)) hide();
    };
    document.addEventListener("keydown", onKey);
    document.addEventListener("mousedown", onDown);
    return () => {
      document.removeEventListener("keydown", onKey);
      document.removeEventListener("mousedown", onDown);
    };
  }, [hide]);

  // ---- Finish: the orb bumps, a ring ripples out and particles spray left.
  const burst = useCallback((color: string) => {
    const o = btn.current, layer = fx.current;
    if (!o || !layer || still()) return;
    o.animate([{ transform: "scale(1)" }, { transform: "scale(1.14)" }, { transform: "scale(.96)" }, { transform: "scale(1)" }], { duration: 480, easing: "ease-out" });
    const ring = document.createElement("span");
    ring.className = "orb-fx";
    Object.assign(ring.style, { width: `${ORB}px`, height: `${ORB}px`, margin: `-${ORB / 2}px 0 0 -${ORB / 2}px`, border: `2px solid ${color}` });
    layer.appendChild(ring);
    ring.animate([{ transform: "scale(1)", opacity: 0.9 }, { transform: "scale(1.75)", opacity: 0 }], { duration: 620, easing: "ease-out" }).onfinish = () => ring.remove();
    for (let i = 0; i < 7; i++) {
      const d = document.createElement("span");
      d.className = "orb-fx";
      const s = 3 + Math.random() * 2.5;
      Object.assign(d.style, { width: `${s}px`, height: `${s}px`, margin: `-${s / 2}px 0 0 -${s / 2}px`, background: color });
      layer.appendChild(d);
      const a = ((140 + i * 13 + Math.random() * 8) * Math.PI) / 180, dist = 30 + Math.random() * 16;
      d.animate([{ transform: "translate(0,0) scale(1)", opacity: 1 }, { transform: `translate(${Math.cos(a) * dist}px,${Math.sin(a) * dist}px) scale(.2)`, opacity: 0 }], { duration: 650 + Math.random() * 200, easing: "cubic-bezier(.2,.8,.3,1)" }).onfinish = () => d.remove();
    }
  }, []);

  // Compare each poll with the last to find personas that just finished.
  const before = useRef<Map<string, DockState> | null>(null);
  const data = dock.data;
  useEffect(() => {
    if (!data) return;
    const found = finishes<Chat, Entry>(before.current, data.personas).filter((f) => f.chat.id !== viewed);
    before.current = chatStates<Chat>(data.personas);
    if (found.length === 0) return;
    for (const f of found) {
      const kind = f.kind === "needs" ? "h" : "d";
      burst(kind === "h" ? "var(--state-human)" : "var(--state-done)");
      setLive(`${f.entry.persona.name} ${kind === "h" ? "needs you" : "finished"}`);
      if (openRef.current) {
        const row = panel.current?.querySelector(`[data-chat="${f.chat.id}"]`);
        const cs = root.current ? getComputedStyle(root.current) : null;
        row?.animate([{ background: cs?.getPropertyValue(kind === "h" ? "--orb-flash-needs" : "--orb-flash-done") || "transparent" }, { background: "transparent" }], { duration: 900 });
        continue;
      }
      setChips((cs) => {
        const next = [...cs, { key: `${f.chat.id}-${Date.now()}`, kind, persona: f.entry.persona, chatId: f.chat.id, title: f.chat.title, leaving: false } satisfies ChipData];
        const keep = next.filter((c) => !c.leaving).slice(-CHIPS_MAX).map((c) => c.key);
        return next.map((c) => (keep.includes(c.key) ? c : { ...c, leaving: true }));
      });
    }
  }, [data, viewed, burst]);

  const openChat = useCallback((id: string) => router.push(`/chat/${id}`), [router]);
  const onChip = useCallback((c: ChipData) => openChat(c.chatId), [openChat]);
  const onGone = useCallback((key: string) => setChips((cs) => cs.filter((c) => c.key !== key)), []);

  const rows = entries.flatMap((e) => e.chats.map((c) => ({ persona: e.persona, chat: c })));
  const working = rows.filter((r) => r.chat.state === "working");
  const badge = orbBadge(entries);
  const quiet = orbQuiet(harnesses, entries);
  const liveGlyph = orbGlyph(harnesses) !== "idle" || working.length > 0;
  const usage = ORB_HARNESSES.flatMap((id) => harnesses.filter((h) => h.harness === id));

  return (
    <div ref={root} role="region" aria-label="Activity" className="orb-root" style={{ bottom: `max(${bottom}px, calc(var(--tab-bar-h, 0px) + 12px))` }}>
      <div className="orb-chips">
        {chips.map((c) => <Chip key={c.key} chip={c} onOpen={onChip} onGone={onGone} />)}
      </div>
      <div ref={panel} className="orb-panel" id="orb-panel" aria-hidden={!open}>
        <div className="orb-sec" data-stg>PERSONAS<span>{working.length} working</span></div>
        <div className="orb-panel-scroll">
          {rows.length === 0 ? (
            <div className="orb-empty" data-stg>No personas working right now</div>
          ) : (
            rows.map(({ persona, chat }) => (
              <button key={chat.id} type="button" className="orb-prow" data-stg data-chat={chat.id} tabIndex={open ? 0 : -1}
                onClick={() => { openChat(chat.id); hide(); }}>
                <PersonaAvatar persona={persona} size={28} />
                <span className="orb-pt"><b>{persona.name}</b><small>{chat.title || "Untitled chat"}</small></span>
                <Status chat={chat} />
              </button>
            ))
          )}
        </div>
        <div className="orb-sec" data-stg>USAGE<span>{checkedAgo(usage)}</span></div>
        {usage.map((h) => <UsageRow key={h.harness} h={h} all={usage} />)}
        <div className="orb-foot" data-stg>
          <Link href="/settings/runner" tabIndex={open ? 0 : -1} onClick={hide}>Usage settings →</Link>
        </div>
      </div>
      <div ref={fx} aria-hidden className="pointer-events-none absolute inset-0 z-[2]" />
      <button ref={btn} type="button" className={`orb-btn${liveGlyph ? " live" : ""}${quiet ? " quiet" : ""}`}
        aria-expanded={open} aria-controls="orb-panel" aria-label={orbLabel(working.map((r) => r.persona.name), entries)}
        onClick={(e) => { e.stopPropagation(); if (openRef.current) hide(); else show(); }}>
        <svg width="44" height="44" viewBox="0 0 44 44" aria-hidden>
          <Ring r={19.5} h={usage.find((h) => h.harness === CLAUDE)} color={RING_COLOR[CLAUDE]} />
          <Ring r={15.5} h={usage.find((h) => h.harness === CODEX)} color={RING_COLOR[CODEX]} />
          <Ring r={11.5} h={usage.find((h) => h.harness === ANTIGRAVITY)} color={RING_COLOR[ANTIGRAVITY]} />
          <Centre harnesses={usage} />
        </svg>
        {badge && <Badge>{badge.kind === "needs" ? "!" : badge.count}</Badge>}
      </button>
      <div className="sr-only" aria-live="polite">{live}</div>
    </div>
  );
}

function Badge({ children }: { children: ReactNode }) {
  return <span aria-hidden className={`orb-badge ${children === "!" ? "h" : "n"}`}>{children}</span>;
}
