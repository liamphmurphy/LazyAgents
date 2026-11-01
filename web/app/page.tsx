import { HealthStatus } from "./health-status";

export default function Home() {
  return (
    <main className="mx-auto flex min-h-dvh max-w-[var(--reading-max)] flex-col justify-center gap-6 px-4">
      <div className="flex items-center gap-3">
        <span aria-hidden className="grid size-7 place-items-center rounded-sm bg-accent text-on-accent type-heading">
          L
        </span>
        <h1 className="type-display text-fg">LazyAgents</h1>
      </div>
      <p className="type-reading text-fg-secondary">
        Scaffold is up. The tracker UI lands in the next phases.
      </p>
      <section className="rounded-xl border border-border bg-surface p-4">
        <h2 className="mb-3 type-heading text-fg">System status</h2>
        <HealthStatus />
      </section>
    </main>
  );
}
