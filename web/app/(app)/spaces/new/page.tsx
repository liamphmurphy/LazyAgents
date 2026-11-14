import { Suspense } from "react";
import { NewSpaceRoute } from "../_components/new-space-route";

// Where "Create a space" links land (LAZYAGENTS-213): the New space dialog over the page, with
// Track work on when ?work=1.
export default function Page() {
  return (
    <Suspense>
      <NewSpaceRoute />
    </Suspense>
  );
}
