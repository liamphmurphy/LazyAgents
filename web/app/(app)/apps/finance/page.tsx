import { Suspense } from "react";
import { FinanceScreen } from "./_components/finance-screen";

export default function Page() {
  // The Ask drawer's conversation reads search params, which stream in Suspense.
  return (
    <Suspense>
      <FinanceScreen />
    </Suspense>
  );
}
