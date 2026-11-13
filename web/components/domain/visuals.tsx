"use client";

import type { ReactNode } from "react";
import { isChartFence, parseChart } from "@/lib/chat-chart";
import { isStepsFence, isSvgFence } from "@/lib/chat-svg";
import { ChartFence, ChartVisual } from "./chart-visual";
import { MermaidFence, isMermaidFence } from "./mermaid-fence";
import { DiagramVisual, StepsFence, SvgFence } from "./svg-fence";

/** The fence types that only draw: safe wherever Markdown shows agent text. */
export function visualFence(info: string | undefined, code: string): ReactNode {
  if (isSvgFence(info)) return <SvgFence code={code} info={info} />;
  if (isStepsFence(info)) return <StepsFence code={code} info={info} />;
  if (isChartFence(info)) return <ChartFence code={code} info={info} />;
  if (isMermaidFence(info)) return <MermaidFence code={code} info={info} />;
  return null;
}

/** A lesson's diagram or chart block, the same renderers as Chat. */
export function LessonVisual({ kind, text, alt, steps, caption }: { kind: "diagram" | "chart"; text: string; alt?: string; steps?: string[]; caption?: ReactNode }) {
  if (kind === "diagram") return <DiagramVisual source={text} captions={steps} alt={alt} label={steps?.length ? "Step-through" : "Diagram"} caption={caption} />;
  const parsed = parseChart(text);
  if ("spec" in parsed) return <ChartVisual spec={parsed.spec} alt={alt} caption={caption} />;
  return <p role="alert" className="rounded-md border border-border p-3 text-12 text-pill-blocked-fg">This chart couldn&apos;t be drawn. {alt}</p>;
}
