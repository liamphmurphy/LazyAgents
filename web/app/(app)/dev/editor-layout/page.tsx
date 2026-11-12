import type { Metadata } from "next";
import { EditorLayoutDemo } from "./demo";

export const metadata: Metadata = { title: "Editor layout · Vesper" };

/** Exercises the shared EditorLayout before any app moves onto it (e2e/editor-layout.spec.ts). */
export default function Page() {
  return <EditorLayoutDemo />;
}
