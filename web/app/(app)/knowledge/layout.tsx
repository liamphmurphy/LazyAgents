import { Suspense } from "react";
import { KnowledgeShell } from "./_components/shell";

export default function KnowledgeLayout({ children }: LayoutProps<"/knowledge">) {
  // The shell reads the pathname, which needs a Suspense boundary for prerendering.
  return (
    <Suspense fallback={null}>
      <KnowledgeShell>{children}</KnowledgeShell>
    </Suspense>
  );
}
