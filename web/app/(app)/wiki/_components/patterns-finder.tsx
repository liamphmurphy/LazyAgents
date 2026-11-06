"use client";

import { useRouter } from "next/navigation";
import { useEffect, useId, useState } from "react";
import { PATTERN_TOPICS, useFindPatterns } from "@/lib/api/hooks";
import { cx } from "@/lib/cx";
import { snippetParts } from "@/lib/snippet";
import { wikiHref } from "@/lib/wikilinks";
import { Banner, Skeleton, TextInput } from "@/components/ui/primitives";

function useDebounced<T>(value: T, ms: number) {
  const [v, setV] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => setV(value), ms);
    return () => clearTimeout(t);
  }, [value, ms]);
  return v;
}

/** Fuzzy search over the pattern library (Patterns/<Topic>/<Title>). */
export function PatternsFinder() {
  const router = useRouter();
  const id = useId();
  const [q, setQ] = useState("");
  const [topic, setTopic] = useState<string>("");
  const [active, setActive] = useState(0);
  const dq = useDebounced(q.trim(), 200);
  const res = useFindPatterns(dq, topic);
  const patterns = res.data ?? [];
  const pending = q.trim() !== dq || res.isPlaceholderData;
  const optId = (i: number) => `${id}-opt-${i}`;
  const open = (path: string) => router.push(wikiHref(path));

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setActive((i) => Math.min(i + 1, patterns.length - 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setActive((i) => Math.max(i - 1, 0));
    } else if (e.key === "Enter" && patterns[active]) {
      e.preventDefault();
      open(patterns[active].path);
    } else if (e.key === "Escape" && q) {
      e.preventDefault();
      setQ("");
    }
  };

  return (
    <section aria-label="Patterns" className="flex flex-col gap-2.5">
      <h2 className="type-heading">Patterns</h2>
      <TextInput
        role="combobox"
        aria-label="Search patterns"
        aria-expanded
        aria-controls={`${id}-list`}
        aria-activedescendant={patterns[active] ? optId(active) : undefined}
        aria-autocomplete="list"
        placeholder="Find a pattern…"
        value={q}
        onChange={(e) => {
          setQ(e.target.value);
          setActive(0);
        }}
        onKeyDown={onKeyDown}
      />
      <div role="group" aria-label="Filter by topic" className="flex flex-wrap gap-1.5">
        {["", ...PATTERN_TOPICS].map((t) => (
          <button
            key={t}
            type="button"
            aria-pressed={topic === t}
            onClick={() => {
              setTopic(t);
              setActive(0);
            }}
            className={cx(
              "h-6 rounded-full border px-2.5 text-xs focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent",
              topic === t ? "border-transparent bg-selected text-fg" : "border-border text-fg-secondary hover:bg-hover",
            )}
          >
            {t || "All"}
          </button>
        ))}
      </div>
      <div aria-live="polite" className="sr-only">
        {res.isSuccess && !pending ? `${patterns.length} patterns` : ""}
      </div>
      {res.isError ? (
        <Banner tone="blocked" title="Couldn't load patterns">
          {res.error.message}
        </Banner>
      ) : !res.data ? (
        <div aria-busy="true" className="flex flex-col gap-2">
          <Skeleton className="h-12 w-full" />
          <Skeleton className="h-12 w-full" />
        </div>
      ) : patterns.length === 0 ? (
        <p className="rounded-xl border border-border bg-surface px-3.5 py-3 text-xs text-fg-tertiary">
          {dq ? "No patterns match." : "No patterns yet."}
        </p>
      ) : (
        <ul
          id={`${id}-list`}
          role="listbox"
          aria-label="Patterns"
          aria-busy={pending}
          className={cx("flex max-h-[420px] flex-col overflow-y-auto rounded-xl border border-border bg-surface", pending && "opacity-70")}
        >
          {patterns.map((p, i) => (
            <li
              key={p.path}
              id={optId(i)}
              role="option"
              aria-selected={i === active}
              onMouseMove={() => i !== active && setActive(i)}
              onClick={() => open(p.path)}
              ref={(el) => {
                if (el && i === active) el.scrollIntoView?.({ block: "nearest" });
              }}
              className={cx(
                "flex cursor-pointer flex-col gap-1 border-b border-border-subtle px-3.5 py-2.5 last:border-0",
                i === active && "bg-row-hover",
              )}
            >
              <span className="flex flex-wrap items-center gap-2">
                <span className="text-[13px] font-medium">{p.title}</span>
                <span className="rounded-full bg-pill-human-bg px-2 text-[11px] text-pill-human-fg">{p.topic}</span>
                {p.tags.map((t) => (
                  <span key={t} className="font-mono text-[11px] text-fg-tertiary">
                    #{t}
                  </span>
                ))}
              </span>
              {p.summary && <span className="text-xs text-fg-secondary">{p.summary}</span>}
              {p.snippet && (
                <span className="line-clamp-2 text-xs text-fg-tertiary">
                  {snippetParts(p.snippet).map((s, j) =>
                    s.match ? (
                      <mark key={j} className="rounded-xs bg-pill-human-bg px-0.5 text-fg">
                        {s.text}
                      </mark>
                    ) : (
                      s.text
                    ),
                  )}
                </span>
              )}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
