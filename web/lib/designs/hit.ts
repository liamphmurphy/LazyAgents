// Hit testing and selection rules for the canvas, on the layout's absolute
// boxes (layout.ts LaidPage.boxes). Pure, so the rules are unit-tested.
import type { DesignNode, Page } from "./schema.ts";
import type { Box } from "./layout.ts";

const inside = (b: Box, x: number, y: number, slop = 0) => x >= b.x - slop && x <= b.x + b.w + slop && y >= b.y - slop && y <= b.y + b.h + slop;
const kids = (n: DesignNode) => ("children" in n && Array.isArray(n.children) ? n.children : []);
/** Containers that are hit on their own box, not only through their children. */
const solid = (n: DesignNode) => n.type !== "group";

/**
 * The nodes under a point, outermost first (a top-level node, then the child
 * under the point inside it, and so on). Topmost in paint order wins.
 */
export function hitPath(page: Page, boxes: ReadonlyMap<string, Box>, x: number, y: number, slop = 0): DesignNode[] {
  const go = (list: DesignNode[]): DesignNode[] => {
    for (let i = list.length - 1; i >= 0; i--) {
      const n = list[i];
      if (n.type === "connector" || n.visible === false || n.locked) continue;
      const b = boxes.get(n.id);
      if (!b || !inside(b, x, y, n.type === "path" ? Math.max(slop, 4) : slop)) continue;
      const deeper = n.type === "instance" ? [] : go(kids(n));
      if (deeper.length || solid(n)) return [n, ...deeper];
    }
    return [];
  };
  return go(page.children);
}

const isBoard = (n: DesignNode) => n.type === "frame" || n.type === "component" || n.type === "section";

/**
 * What a click selects: like design tools, a top-level frame's (or section's)
 * contents are picked directly, the board itself when its empty area is
 * clicked. Inside a current selection, a click goes one level deeper
 * (double-click), and deep (⌘-click) picks the innermost node.
 */
export function pick(path: DesignNode[], selected: readonly string[], mode: "click" | "deeper" | "deep"): DesignNode | undefined {
  if (!path.length) return undefined;
  if (mode === "deep") return path.at(-1);
  if (mode === "deeper") {
    const i = path.findIndex((n) => selected.includes(n.id));
    if (i >= 0) return path[Math.min(i + 1, path.length - 1)];
  }
  // A click on something already selected keeps it (so dragging a selection works).
  const sel = path.find((n) => selected.includes(n.id));
  if (sel && mode === "click") return sel;
  // A selected sibling's parent scope: clicking next to the selection stays at its depth.
  for (let d = path.length - 1; d > 0; d--) {
    const parent = path[d - 1];
    if (selected.length && kids(parent).some((k) => selected.includes(k.id))) return path[d];
  }
  return isBoard(path[0]) && path.length > 1 ? path[1] : path[0];
}

/**
 * What a connector drawn from (or to) a point attaches to: the innermost node
 * under it that isn't a section or group (those are areas, not diagram nodes),
 * else the section itself.
 */
export const connectTarget = (path: DesignNode[]): DesignNode | undefined => [...path].reverse().find((n) => n.type !== "section" && n.type !== "group") ?? path.at(-1);

/** Top-level-or-child nodes wholly inside a marquee (page coordinates), at the depth the marquee started in. */
export function inMarquee(list: DesignNode[], boxes: ReadonlyMap<string, Box>, m: Box): string[] {
  const out: string[] = [];
  for (const n of list) {
    if (n.type === "connector" || n.visible === false || n.locked) continue;
    const b = boxes.get(n.id);
    if (!b) continue;
    if (b.x >= m.x && b.y >= m.y && b.x + b.w <= m.x + m.w && b.y + b.h <= m.y + m.h) out.push(n.id);
  }
  return out;
}

/** The innermost container under a point that new nodes go into (a frame, component or section), if any. */
export function dropTarget(page: Page, boxes: ReadonlyMap<string, Box>, x: number, y: number, exclude: ReadonlySet<string> = new Set()): DesignNode | undefined {
  let found: DesignNode | undefined;
  const go = (list: DesignNode[]) => {
    for (let i = list.length - 1; i >= 0; i--) {
      const n = list[i];
      if (exclude.has(n.id) || n.visible === false || !isBoard(n)) continue;
      const b = boxes.get(n.id);
      if (!b || !inside(b, x, y)) continue;
      found = n;
      go(kids(n));
      return;
    }
  };
  go(page.children);
  return found;
}
