// Actions revealed on hover must also show where there is no hover (touch
// screens), or they can't be found. Keep every hover reveal on these helpers.
// The class strings are literal so Tailwind's source scan picks them up.

/** The Tailwind group a reveal listens to: `group`, `group/msg` or `group/view`. */
export type RevealGroup = "" | "msg" | "view";

const SHOW: Record<RevealGroup, string> = {
  "": "group-hover:opacity-100 [@media(hover:none)]:opacity-100",
  msg: "group-hover/msg:opacity-100 [@media(hover:none)]:opacity-100",
  view: "group-hover/view:opacity-100 [@media(hover:none)]:opacity-100",
};

/** Classes that bring an element to full opacity on group hover, or always without hover. */
export function hoverShow(group: RevealGroup = ""): string {
  return SHOW[group];
}

/** Like hoverReveal, for elements hidden with `invisible` so they also leave the tab order. */
export function hoverVisible(): string {
  return "invisible group-hover:visible [@media(hover:none)]:visible";
}

/** Hidden until the group is hovered; always shown on devices without hover. */
export function hoverReveal(group: RevealGroup = ""): string {
  return `opacity-0 ${SHOW[group]}`;
}
