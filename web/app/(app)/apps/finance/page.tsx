import { Suspense } from "react";
import { FinanceScreen } from "./_components/finance-screen";

export default function Page() {
  // The Chat tab's conversation reads search params, which stream in Suspense.
  return (
    <Suspense>
      <FinanceScreen />
    </Suspense>
  );
}
