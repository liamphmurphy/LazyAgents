"use client";

import { DropdownMenu } from "radix-ui";
import type { Schemas } from "@/lib/api/client";
import { cx } from "@/lib/cx";
import { Icon } from "@/components/ui/icon";
import { floatPanel } from "@/components/domain/pickers";

type Project = Schemas["Project"];

/** Picks the project whose override the agent tabs preview. */
export function ProjectPicker({
  projects,
  value,
  onChange,
  label,
  className,
}: {
  projects: Project[];
  value?: string;
  onChange: (key: string) => void;
  label: string;
  className?: string;
}) {
  const p = projects.find((x) => x.key === value);
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button
          type="button"
          aria-label={`${label}: ${p?.name ?? "no project"}`}
          className={cx("flex h-7 items-center gap-2 rounded-md border border-border-strong px-2 text-[12.5px] hover:bg-hover", className)}
        >
          {p && <span aria-hidden className="size-2.5 rounded-[3px]" style={{ background: p.color }} />}
          <span className="flex-1 truncate text-left">{p?.name ?? "Pick a project"}</span>
          <Icon name="chev" size={12} className="text-fg-tertiary" />
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align="start" sideOffset={4} className={cx(floatPanel, "w-56")}>
          {projects.map((x) => (
            <DropdownMenu.Item
              key={x.key}
              onSelect={() => onChange(x.key)}
              className="flex h-8 items-center gap-2.5 rounded-md px-2 outline-none data-[highlighted]:bg-hover"
            >
              <span aria-hidden className="size-2.5 rounded-[3px]" style={{ background: x.color }} />
              {x.name}
            </DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}
