"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useCallback, useEffect, useLayoutEffect, useRef, useState, useSyncExternalStore, type ReactNode } from "react";
import { PersonaAvatar } from "@/app/(app)/chat/_components/persona";
import type { Schemas } from "@/lib/api/client";
import { useMarkChatRead, usePersonaDock } from "@/lib/api/hooks";
import { CLAUDE, CODEX, checkedAgo, chatStates, finishes, orbBadge, orbGlyph, orbLabel, orbQuiet, sessionPercent, usageSub, type HarnessActivity } from "@/lib/activity-orb";
import { dockEntries, viewedChatId, type DockState } from "@/lib/persona-dock";

type Entry = Schemas["PersonaDockEntry"];
type Chat = Schemas["PersonaDockChat"];
type Persona = Entry["persona"];

/*
 * The one floating element: harness activity (Claude's spark, Codex's comets),
 * the two session-usage rings, and a badge for personas that need the person
 * or replied. Clicking it sprouts a panel out to the left; a persona that
 * finishes sends a chip out of it. The motion is a straight port of the
 * approved mockup: keep its keyframes, timings and easings in step with it.
 */

const ORB = 46;
const CHIP_DONE_MS = 4500;
const CHIP_NEEDS_MS = 8000;
const CHIP_LINGER_MS = 2000;
const CHIPS_MAX = 2;

/** Clears bottom bars and floating buttons (bulk bar, jump-to-latest). */
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

// ---- Glyphs: the mockup's ring(), spark(), orbit() and idle(), in a 44×44 box.

function Ring({ r, pct, red }: { r: number; pct: number | null; red: boolean }) {
  const c = 2 * Math.PI * r;
  return (
    <>
      <circle cx="22" cy="22" r={r} fill="none" stroke="var(--orb-track)" strokeWidth="2.4" />
      {pct !== null && pct > 0 && (
        <circle cx="22" cy="22" r={r} fill="none" stroke={red ? "var(--state-blocked)" : "var(--orb-ring)"} strokeWidth="2.4" strokeLinecap="round"
          strokeDasharray={`${(c * pct) / 100} ${c}`} transform="rotate(-90 22 22)" />
      )}
    </>
  );
}

function Spark({ len }: { len: number }) {
  return (
    <g className="orb-spin">
      <g className="orb-breathe" stroke="var(--orb-claude)" strokeWidth="2.3" strokeLinecap="round">
        {[0, 1, 2, 3, 4, 5].map((i) => {
          const a = (i * Math.PI) / 3;
          return <line key={i} x1="22" y1="22" x2={22 + len * Math.cos(a)} y2={22 + len * Math.sin(a)} />;
        })}
      </g>
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

function Centre({ harnesses }: { harnesses: HarnessActivity[] }) {
  const g = orbGlyph(harnesses);
  if (g === "both") return <><Spark len={4.2} /><Orbit r={9.5} /></>;
  if (g === "claude") return <Spark len={7} />;
  if (g === "codex") return <Orbit r={7.5} />;
  return <Idle />;
}

function Mini({ harness }: { harness: string }) {
  return (
    <svg width="20" height="20" viewBox="0 0 44 44" aria-hidden>
      {harness === CODEX ? <Orbit r={10} /> : <Spark len={10} />}
    </svg>
  );
}

// ---- Panel rows

function Status({ chat }: { chat: Chat }) {
  if (chat.state === "working") return <span className="orb-ps"><Mini harness={chat.harness} />Working</span>;
  if (chat.state === "needs_you") return <span className="orb-ps h">Needs you</span>;
  return <span className="orb-ps"><span className="orb-dot" />Done{chat.unread ? ` · ${chat.unread} new` : ""}</span>;
}

function UsageRow({ h, all }: { h: HarnessActivity; all: HarnessActivity[] }) {
  const pct = sessionPercent(h);
  const red = !!h.limited_until;
  return (
    <>
      <div className="orb-urow" data-stg>
        <span>{h.label}</span>
        <div className="orb-track"><i className={red ? "red" : ""} style={{ width: `${pct ?? 0}%` }} /></div>
        <span className="orb-val">{pct === null ? "—" : `${pct}%`}</span>
      </div>
      <div className={`orb-usub${red ? " red" : ""}`} data-stg>{usageSub(h, all)}</div>
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

/** Which personas and harnesses are working, what's left of each plan, and who finished, on every screen. */
export function ActivityOrb() {
  const router = useRouter();
  const pathname = usePathname();
  const dock = usePersonaDock();
  const mark = useMarkChatRead();
  const bottom = useOrbBottom();
  const visible = useSyncExternalStore(subscribeVisible, () => document.visibilityState === "visible", () => false);
  const viewed = visible ? viewedChatId(pathname) : null;
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

  // Opening a chat reads it, and so does a reply arriving while it's on screen.
  const unreadViewed = dock.data?.personas.flatMap((e) => e.chats).find((c) => c.id === viewed && c.unread > 0);
  const { mutate } = mark;
  const seq = unreadViewed?.last_seq;
  useEffect(() => {
    if (viewed && seq) mutate({ id: viewed, seq });
  }, [viewed, seq, mutate]);

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
  const usage = [CLAUDE, CODEX].flatMap((id) => harnesses.filter((h) => h.harness === id));

  return (
    <div ref={root} role="region" aria-label="Activity" className="orb-root" style={{ bottom }}>
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
          <Ring r={19.5} pct={sessionPercent(usage.find((h) => h.harness === CLAUDE))} red={!!usage.find((h) => h.harness === CLAUDE)?.limited_until} />
          <Ring r={15.5} pct={sessionPercent(usage.find((h) => h.harness === CODEX))} red={!!usage.find((h) => h.harness === CODEX)?.limited_until} />
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
