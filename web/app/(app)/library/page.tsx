import { Suspense } from "react";
import { LibraryScreen } from "./library-screen";

export default function Page() {
  return (
    <Suspense>
      <LibraryScreen />
    </Suspense>
  );
}
