"use client";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AppearanceSync } from "@/components/shell/theme";
import { ConfirmHost } from "@/components/ui/confirm";
import { Toaster } from "@/components/ui/toast";
import { useState } from "react";

export function Providers({ children }: { children: React.ReactNode }) {
  // One client per browser session. SSE invalidations (Phase 6) target it.
  const [client] = useState(
    () => new QueryClient({ defaultOptions: { queries: { staleTime: 30_000 } } }),
  );
  return <QueryClientProvider client={client}><AppearanceSync />{children}<Toaster /><ConfirmHost /></QueryClientProvider>;
}
