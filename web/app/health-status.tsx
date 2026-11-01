"use client";

import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api/client";

export function HealthStatus() {
  const { data, isPending, isError } = useQuery({
    queryKey: ["health"],
    queryFn: async () => {
      // 503 still carries a Health body describing which dependency failed.
      const { data, error } = await api.GET("/health");
      return data ?? error ?? null;
    },
    refetchInterval: 10_000,
  });

  if (isPending) return <p className="type-small text-fg-tertiary">Checking API…</p>;
  if (isError || !data || !("checks" in data)) {
    return <p className="type-small text-state-blocked">API unreachable</p>;
  }

  return (
    <dl className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-2 type-small">
      <dt className="text-fg-tertiary">API</dt>
      <dd className={data.status === "ok" ? "text-state-done" : "text-state-blocked"}>
        {data.status} <span className="type-mono text-fg-tertiary">· {data.version}</span>
      </dd>
      {Object.entries(data.checks).map(([name, c]) => (
        <div key={name} className="contents">
          <dt className="text-fg-tertiary">{name}</dt>
          <dd className={c.ok ? "text-state-done" : "text-state-blocked"}>
            {c.ok ? "ok" : c.error}
            {c.latency_ms != null && <span className="type-mono text-fg-tertiary"> · {c.latency_ms}ms</span>}
          </dd>
        </div>
      ))}
    </dl>
  );
}
