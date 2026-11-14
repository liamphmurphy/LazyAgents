"use client";

import { useEffect, useSyncExternalStore, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { claimNavigator, navigatorSlot, subscribeNavigator } from "@/lib/navigator";

/**
 * Puts a screen's own list (the Runs list, the Knowledge explorer, an editor's pages and layers)
 * in the left column's middle, in place of Chats, so the screen draws no second column. It renders
 * nothing while the column is collapsed or hidden, as the column's other lists don't show then.
 * The content keeps this screen's React context, so it can share state with the page.
 */
export function NavigatorPortal({ children }: { children: ReactNode }) {
  const slot = useSyncExternalStore(subscribeNavigator, navigatorSlot, () => null);
  useEffect(() => claimNavigator(), []);
  return slot ? createPortal(children, slot) : null;
}
