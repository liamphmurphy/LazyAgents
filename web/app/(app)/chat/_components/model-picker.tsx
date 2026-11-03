"use client";

import { DropdownMenu } from "radix-ui";
import type { Harness, Schemas } from "@/lib/api/client";
import { cx } from "@/lib/cx";
import { Icon } from "@/components/ui/icon";
import { floatPanel } from "@/components/domain/pickers";

export type Selection = { harness: Harness; model: string };
type Group = Schemas["ChatHarnessModels"];

export const sameSelection = (a: Selection, b: Selection) => a.harness === b.harness && a.model === b.model;

export function modelLabel(groups: Group[] | undefined, s: Selection) {
  const g = groups?.find((x) => x.harness === s.harness);
  return { harness: g?.label ?? s.harness, model: g?.models.find((m) => m.id === s.model)?.label ?? s.model };
}

const item =
  "flex min-h-8 cursor-default select-none flex-col justify-center rounded-md px-2 py-1 outline-none data-[highlighted]:bg-hover data-[disabled]:opacity-50";

/** Harness/model choice for the next message (applies from that message on). */
export function ModelPicker({
  groups,
  value,
  onChange,
  disabled,
}: {
  groups?: Group[];
  value: Selection;
  onChange: (s: Selection) => void;
  disabled?: boolean;
}) {
  const l = modelLabel(groups, value);
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button
          type="button"
          disabled={disabled}
          aria-label={`Model for the next message: ${l.harness}, ${l.model}`}
          className="flex h-[30px] max-w-full items-center gap-1.5 rounded-md border border-border-strong bg-btn-outline px-2.5 text-[13px] text-fg-2 hover:bg-hover disabled:cursor-not-allowed disabled:opacity-50"
        >
          <span className="truncate font-mono text-xs">{l.harness} · {l.model}</span>
          <Icon name="chev" size={13} strokeWidth={1.6} className="text-fg-tertiary" />
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align="start" side="top" sideOffset={6} className={cx(floatPanel, "w-64")}>
          {(groups ?? []).map((g, i) => (
            <DropdownMenu.Group key={g.harness} aria-label={g.label}>
              {i > 0 && <DropdownMenu.Separator className="my-1 h-px bg-border-subtle" />}
              <DropdownMenu.Label className="px-2 py-1 text-xs font-medium text-fg-tertiary">{g.label}</DropdownMenu.Label>
              {g.models.map((m) => {
                const on = value.harness === g.harness && value.model === m.id;
                return (
                  <DropdownMenu.Item
                    key={m.id}
                    disabled={!g.available}
                    aria-label={`${g.label} ${m.label}${g.available ? "" : ` (unavailable: ${g.unavailable_reason ?? "not configured"})`}`}
                    className={item}
                    onSelect={() => onChange({ harness: g.harness, model: m.id })}
                  >
                    <span className="flex items-center gap-2">
                      {m.label}
                      <span className="ml-auto w-4 text-accent">{on && <Icon name="check" size={14} strokeWidth={1.8} />}</span>
                    </span>
                    {!g.available && <span className="text-xs text-fg-tertiary">{g.unavailable_reason ?? "Unavailable"}</span>}
                  </DropdownMenu.Item>
                );
              })}
            </DropdownMenu.Group>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}
