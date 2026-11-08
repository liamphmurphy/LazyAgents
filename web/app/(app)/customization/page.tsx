import { Suspense } from "react";
import { CustomizationScreen } from "./customization-screen";

export default function Page() {
  return (
    <Suspense>
      <CustomizationScreen />
    </Suspense>
  );
}
