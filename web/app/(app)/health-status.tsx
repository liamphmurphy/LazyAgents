"use client";

import { useQuery } from "@tanstack/react-query";
import { api, type Schemas } from "@/lib/api/client";

type Health = Schemas["Health"];

function isHealth(v: unknown): v is Health {
  return typeof v === "object" && v !== null && "checks" in v && "status" in v;
}

export function HealthStatus() {
  const { data, isPending, error } = useQuery({
    queryKey: ["health"],
    queryFn: async (): Promise<Health> => {
      const { data, error, response } = await api.GET("/health");
      // 503 still carries a Health body describing which dependency failed.
      const body = data ?? error;
      if (isHealth(body)) return body;
      // Anything else (proxy error page, plain-text 502, empty body) means
      // the API itself didn't answer.
      throw new Error(`HTTP ${response.status}${response.statusText ? ` ${response.statusText}` : ""}`);
    },
    refetchInterval: 10_000,
    retry: false,
  });

  if (isPending) return <p className="type-small text-fg-tertiary">Checking API…</p>;
  if (!data) {
    return (
      <p className="type-small text-state-blocked">
        API unreachable{error && <span className="type-mono text-fg-tertiary"> · {error.message}</span>}
      </p>
    );
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
