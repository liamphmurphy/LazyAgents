/**
 * The shell's two left columns (the workbench layout): the icon rail is always there and switches
 * areas; beside it, the navigator lists what the current area holds. Pure so the rules are
 * unit-tested; the components are components/shell/sidebar.tsx and the areas' own lists.
 */
import { isActive } from "./project-scope.ts";

/**
 * Every navigator has the same width, whoever draws it: the Chats list, the Runs list, the
 * Knowledge explorer and an editor's structure panel. A literal class so Tailwind finds it.
 */
export const NAVIGATOR_WIDTH = "w-[264px]";

/** Areas whose navigator the shell draws. Runs, Knowledge and the app editors draw their own in the same place. */
export type NavigatorArea = "chats";

/** The area whose list the shell's navigator shows on a path, or null when the page brings its own or has none. */
export function navigatorArea(pathname: string): NavigatorArea | null {
  if (pathname === "/" || isActive(pathname, "/chat") || isActive(pathname, "/spaces")) return "chats";
  return null;
}

/** The rail's Chat entry reads as current on any page whose navigator is the Chats list. */
export function inChats(pathname: string): boolean {
  return navigatorArea(pathname) === "chats";
}
