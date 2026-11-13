"use client";

import { useRouter, useSearchParams } from "next/navigation";
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

const chip = (on: boolean) =>
  cx(
    "h-6 rounded-full border px-2.5 text-12 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent",
    on ? "border-transparent bg-selected text-fg" : "border-border text-fg-secondary hover:bg-hover",
  );

/**
 * Fuzzy search over the pattern library (Patterns/<Topic>/<Title>), filtered
 * by topic and tags. Topic and tags live in the URL (?topic=Go&tag=mcp&tag=runner)
 * so a filtered index can be shared; several tags must all match.
 */
export function PatternsFinder({ index = false }: { index?: boolean }) {
  const router = useRouter();
  const search = useSearchParams();
  const id = useId();
  const [q, setQ] = useState("");
  const topic = search.get("topic") ?? "";
  const tags = search.getAll("tag").map((t) => t.toLowerCase());
  const [active, setActive] = useState(0);
  const [moreTags, setMoreTags] = useState(false);
  const dq = useDebounced(q.trim(), 200);
  const res = useFindPatterns(dq, topic, tags);
  const patterns = res.data?.patterns ?? [];
  const facet = res.data?.tags ?? [];
  const pending = q.trim() !== dq || res.isPlaceholderData;
  const optId = (i: number) => `${id}-opt-${i}`;
  const open = (path: string) => router.push(wikiHref(path));

  const setFilters = (next: { topic?: string; tags?: string[] }) => {
    const p = new URLSearchParams(search.toString());
    const t = next.topic ?? topic;
    if (t) p.set("topic", t);
    else p.delete("topic");
    p.delete("tag");
    for (const x of next.tags ?? tags) p.append("tag", x);
    const qs = p.toString();
    router.replace(qs ? `/wiki?${qs}` : "/wiki", { scroll: false });
    setActive(0);
  };
  const toggleTag = (t: string) => setFilters({ tags: tags.includes(t) ? tags.filter((x) => x !== t) : [...tags, t] });
  // Selected tags first (even when the topic has none of them), then the facet by count.
  const counts = new Map(facet.map((f) => [f.tag, f.count]));
  const chips = [...tags.map((t) => ({ tag: t, count: counts.get(t) ?? 0 })), ...facet.filter((f) => !tags.includes(f.tag))];
  const visibleChips = moreTags ? chips : chips.slice(0, Math.max(6, tags.length));

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
  const tagList = tags.map((t) => `#${t}`).join(" ");

  return (
    <section aria-label="Patterns" className="flex flex-col gap-2.5">
      {index ? (
        <h1 className="type-display">
          Patterns{" "}
          {res.data && !dq && !topic && !tags.length && patterns.length < 50 && <span className="text-14 font-normal text-fg-tertiary">{patterns.length}</span>}
        </h1>
      ) : (
        <h2 className="type-heading">Patterns</h2>
      )}
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
      <div role="group" aria-label="Filter by topic" className="flex flex-wrap items-center gap-1.5">
        <span className="w-9 text-12 text-fg-tertiary">Topic</span>
        {["", ...PATTERN_TOPICS].map((t) => (
          <button key={t} type="button" aria-pressed={topic === t} onClick={() => setFilters({ topic: t })} className={chip(topic === t)}>
            {t || "All"}
          </button>
        ))}
      </div>
      {chips.length > 0 && (
        <div role="group" aria-label="Filter by tag" className="flex flex-wrap items-center gap-1.5">
          <span className="w-9 text-12 text-fg-tertiary">Tag</span>
          {visibleChips.map((f) => {
            const on = tags.includes(f.tag);
            return (
              <button
                key={f.tag}
                type="button"
                aria-pressed={on}
                onClick={() => toggleTag(f.tag)}
                className={cx(chip(false), "font-mono text-12", on && "border-accent bg-pill-human-bg text-pill-human-fg hover:bg-pill-human-bg")}
              >
                {f.tag}
                {on && " ✓"} <span className={on ? "" : "text-fg-tertiary"}>{f.count}</span>
              </button>
            );
          })}
          {chips.length > visibleChips.length && (
            <button type="button" className="text-12 text-accent hover:underline" onClick={() => setMoreTags(true)}>
              + {chips.length - visibleChips.length} more
            </button>
          )}
          {moreTags && chips.length > 6 && (
            <button type="button" className="text-12 text-accent hover:underline" onClick={() => setMoreTags(false)}>
              Fewer
            </button>
          )}
        </div>
      )}
      {tags.length > 0 && res.data && (
        <p className="text-12 text-fg-secondary">
          {patterns.length} {patterns.length === 1 ? "pattern" : "patterns"} tagged <span className="font-mono">{tagList}</span> ·{" "}
          <button type="button" className="text-accent hover:underline" onClick={() => setFilters({ tags: [] })}>
            Clear
          </button>
        </p>
      )}
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
        <p className="rounded-xl border border-border bg-surface px-3.5 py-3 text-12 text-fg-tertiary">
          {tags.length
            ? `No patterns tagged ${tagList}${topic ? ` in ${topic}` : ""}${dq ? " match" : ""}.`
            : dq
              ? "No patterns match."
              : topic
                ? `No patterns in ${topic} yet.`
                : "No patterns yet."}
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
                <span className="text-13 font-medium">{p.title}</span>
                <span className="rounded-full bg-pill-human-bg px-2 text-11 text-pill-human-fg">{p.topic}</span>
                {p.tags.map((t) => (
                  <span key={t} className={cx("font-mono text-11", tags.includes(t) ? "font-semibold text-pill-human-fg" : "text-fg-tertiary")}>
                    #{t}
                  </span>
                ))}
              </span>
              {p.summary && <span className="text-12 text-fg-secondary">{p.summary}</span>}
              {p.snippet && (
                <span className="line-clamp-2 text-12 text-fg-tertiary">
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
