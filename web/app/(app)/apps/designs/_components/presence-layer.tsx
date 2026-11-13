"use client";

import { useEffect, useState } from "react";
import type { Box } from "@/lib/designs/edit";
import type { Camera } from "./design-canvas";

const PRESENCE = "#3FC1B0";
/** The cursor visits each node a patch touched, in order, but a big patch's walk stays short. */
const STEP_MS = 350;
const WALK_MS = 2400;

/**
 * A persona's selection outline over the nodes it is working on, and its named cursor, which
 * moves from node to node in the order it touched them and glides between them.
 */
export function PresenceLayer({ name, nodes, camera, busy }: { name: string; nodes: { id: string; box: Box }[]; camera: Camera; busy: boolean }) {
  // A new set of nodes starts the walk again from the first.
  const walkKey = nodes.map((n) => n.id).join(" ");
  const [walk, setWalk] = useState({ key: walkKey, at: 0 });
  if (walk.key !== walkKey) setWalk({ key: walkKey, at: 0 });
  const last = nodes.length - 1;
  useEffect(() => {
    if (walk.at >= last) return;
    const t = setTimeout(() => setWalk((w) => ({ ...w, at: w.at + 1 })), Math.min(STEP_MS, WALK_MS / last));
    return () => clearTimeout(t);
  }, [walk, last]);
  if (!nodes.length) return null;
  const at = nodes[Math.min(walk.at, last)].box;
  return (
    <svg aria-hidden className="pointer-events-none absolute inset-0 z-[6] h-full w-full" data-testid="persona-presence">
      {nodes.map(({ id, box: b }) => (
        <rect key={id} x={b.x * camera.z + camera.x - 2} y={b.y * camera.z + camera.y - 2} width={b.w * camera.z + 4} height={b.h * camera.z + 4} rx={3} fill="none" stroke={PRESENCE} strokeWidth={1.5} strokeDasharray={busy ? "5 3" : undefined} />
      ))}
      {/* Page coordinates under the camera, so the glide animates the persona's moves and not the person's pans. */}
      <g transform={`translate(${camera.x} ${camera.y}) scale(${camera.z})`}>
        <g data-testid="persona-cursor" className="motion-safe:transition-transform motion-safe:duration-300 motion-safe:ease-out" style={{ transform: `translate(${at.x + at.w}px, ${at.y + at.h}px)` }}>
          <g transform={`scale(${1 / camera.z}) translate(4 4)`}>
            <path d="M0 0L0 14L4 10.5L7 17L9.5 16L6.5 9.5L12 9.5Z" fill={PRESENCE} stroke="#0C0B12" strokeWidth={1} />
            <rect x={12} y={14} width={name.length * 6.5 + 14} height={18} rx={9} fill={PRESENCE} />
            <text x={19} y={26.5} fontSize={11} fontWeight={600} fill="#0C0B12">
              {name}
            </text>
          </g>
        </g>
      </g>
    </svg>
  );
}
