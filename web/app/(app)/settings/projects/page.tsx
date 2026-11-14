import { Suspense } from "react";
import { ProjectsRedirect } from "./projects-redirect";

// Projects are spaces that track work, managed on the space's page (LAZYAGENTS-213).
export default function Page() {
  return (
    <Suspense>
      <ProjectsRedirect />
    </Suspense>
  );
}
