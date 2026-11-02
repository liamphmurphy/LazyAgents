import { Suspense } from "react";
import { ArtifactsScreen } from "./_components/artifacts-screen";

export default function ArtifactsPage() {
  return (
    <Suspense>
      <ArtifactsScreen />
    </Suspense>
  );
}
