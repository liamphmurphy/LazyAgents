"use client";

import type { Box } from "@/lib/designs/edit";
import type { Camera } from "./design-canvas";

const PRESENCE = "#3FC1B0";

/** A persona's named cursor and selection outline over the nodes it is working on (screen space, from the canvas camera). */
export function PresenceLayer({ name, boxes, camera, busy }: { name: string; boxes: Box[]; camera: Camera; busy: boolean }) {
  if (!boxes.length) return null;
  const s = boxes.map((b) => ({ x: b.x * camera.z + camera.x, y: b.y * camera.z + camera.y, w: b.w * camera.z, h: b.h * camera.z }));
  const right = Math.max(...s.map((b) => b.x + b.w)), bottom = Math.max(...s.map((b) => b.y + b.h));
  return (
    <svg aria-hidden className="pointer-events-none absolute inset-0 z-[6] h-full w-full" data-testid="persona-presence">
      {s.map((b, i) => (
        <rect key={i} x={b.x - 2} y={b.y - 2} width={b.w + 4} height={b.h + 4} rx={3} fill="none" stroke={PRESENCE} strokeWidth={1.5} strokeDasharray={busy ? "5 3" : undefined} />
      ))}
      <g transform={`translate(${right + 4} ${bottom + 4})`}>
        <path d="M0 0L0 14L4 10.5L7 17L9.5 16L6.5 9.5L12 9.5Z" fill={PRESENCE} stroke="#0C0B12" strokeWidth={1} />
        <rect x={12} y={14} width={name.length * 6.5 + 14} height={18} rx={9} fill={PRESENCE} />
        <text x={19} y={26.5} fontSize={11} fontWeight={600} fill="#0C0B12">
          {name}
        </text>
      </g>
    </svg>
  );
}
