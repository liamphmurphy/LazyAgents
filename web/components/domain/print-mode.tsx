"use client";

import { createContext, useContext } from "react";

/**
 * True inside a print view (Export PDF). Visuals use it to show what paper can
 * hold: a step-through prints its last frame with every caption, for example.
 */
export const PrintMode = createContext(false);

export const usePrintMode = () => useContext(PrintMode);

/** Where a markdown artifact prints from; the page opens the print dialog itself. */
export const printHref = (artifactId: string, versionId?: string | null) =>
  `/print/artifacts/${artifactId}${versionId ? `?version=${versionId}` : ""}`;
