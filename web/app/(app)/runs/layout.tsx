import { Suspense } from "react";
import { RunsLayout } from "./runs-layout";

/** /runs and /runs/[id] share the run list, so it stays mounted while runs are opened beside it. */
export default function Layout({ children }: LayoutProps<"/runs">) {
  return (
    <Suspense>
      <RunsLayout>{children}</RunsLayout>
    </Suspense>
  );
}
