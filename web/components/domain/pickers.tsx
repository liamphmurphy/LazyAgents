"use client";

import { DropdownMenu, Popover } from "radix-ui";
import { useMemo, useState, type ReactNode } from "react";
import type { Priority, Schemas, TicketStatus } from "@/lib/api/client";
import { cx } from "@/lib/cx";
import { Icon } from "@/components/ui/icon";
import { AgentAvatar, HumanAvatar, UnassignedAvatar } from "./avatars";
import { PriorityGlyph, StatusGlyph } from "./glyphs";
import { PRIORITY_NAME, PRIORITY_ORDER, STATUS_NAME, STATUS_ORDER } from "./meta";

// Floating UI: raised surface + float border + shadow (DESIGN.md §1).
export const floatPanel =
  "z-50 rounded-xl border border-border-float bg-surface-raised p-1 text-[13px] text-fg shadow-[var(--shadow-float)] outline-none";
const item =
  "flex h-8 cursor-default select-none items-center gap-2.5 rounded-md px-2 outline-none data-[highlighted]:bg-hover data-[disabled]:opacity-50";

function Check({ on }: { on: boolean }) {
  return <span className="ml-auto w-4 text-accent">{on && <Icon name="check" size={14} strokeWidth={1.8} />}</span>;
}

type MenuProps<T> = { value?: T; onChange: (v: T) => void; children: ReactNode; align?: "start" | "end" };

export function StatusMenu({ value, onChange, children, align = "start", statuses = STATUS_ORDER }: MenuProps<TicketStatus> & { statuses?: TicketStatus[] }) {
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>{children}</DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align={align} sideOffset={6} className={cx(floatPanel, "w-52")}>
          {statuses.map((s) => (
            <DropdownMenu.Item key={s} className={item} onSelect={() => onChange(s)}>
              <StatusGlyph state={s} surface="var(--surface-raised)" label={false} />
              {STATUS_NAME[s]}
              <Check on={s === value} />
            </DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}

export function PriorityMenu({ value, onChange, children, align = "start" }: MenuProps<Priority>) {
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>{children}</DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align={align} sideOffset={6} className={cx(floatPanel, "w-48")}>
          {PRIORITY_ORDER.map((p) => (
            <DropdownMenu.Item key={p} className={item} onSelect={() => onChange(p)}>
              <PriorityGlyph priority={p} label={false} />
              {PRIORITY_NAME[p]}
              <Check on={p === value} />
            </DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}

/** Choose whether tickets need a person-approved mockup (bulk edits). */
export function MockupRequirementMenu({ onChange, children, align = "start" }: Omit<MenuProps<boolean>, "value">) {
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>{children}</DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align={align} sideOffset={6} className={cx(floatPanel, "w-52")}>
          <DropdownMenu.Item className={item} onSelect={() => onChange(true)}>
            Require mockup
          </DropdownMenu.Item>
          <DropdownMenu.Item className={item} onSelect={() => onChange(false)}>
            Don’t require mockup
          </DropdownMenu.Item>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}

export function UserMenu({
  users,
  value,
  onChange,
  children,
}: MenuProps<string | null> & { users: Schemas["User"][] }) {
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>{children}</DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align="start" sideOffset={6} className={cx(floatPanel, "w-56")}>
          {users.map((u) => (
            <DropdownMenu.Item key={u.id} className={item} onSelect={() => onChange(u.id)}>
              <HumanAvatar name={u.name} initials={u.initials} color={u.avatar_color} label={false} />
              {u.name}
              <Check on={u.id === value} />
            </DropdownMenu.Item>
          ))}
          <DropdownMenu.Separator className="my-1 h-px bg-border-subtle" />
          <DropdownMenu.Item className={cx(item, "text-fg-secondary")} onSelect={() => onChange(null)}>
            No reviewer
          </DropdownMenu.Item>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}

export function EpicMenu({
  epics,
  value,
  onChange,
  children,
}: MenuProps<string | null> & { epics: Schemas["Epic"][] }) {
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>{children}</DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align="start" sideOffset={6} className={cx(floatPanel, "w-56")}>
          {epics.length === 0 && <div className="px-2 py-1.5 text-fg-tertiary">No epics in this project</div>}
          {epics.map((e) => (
            <DropdownMenu.Item key={e.id} className={item} onSelect={() => onChange(e.id)}>
              <Icon name="epics" size={14} className="text-fg-tertiary" />
              {e.name}
              <Check on={e.id === value} />
            </DropdownMenu.Item>
          ))}
          <DropdownMenu.Separator className="my-1 h-px bg-border-subtle" />
          <DropdownMenu.Item className={cx(item, "text-fg-secondary")} onSelect={() => onChange(null)}>
            No epic
          </DropdownMenu.Item>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}

const availability: Record<Schemas["Agent"]["status"], { label: string; cls: string }> = {
  idle: { label: "Available", cls: "text-pill-done-fg" },
  running: { label: "Busy", cls: "text-pill-human-fg" },
  waiting: { label: "Waiting", cls: "text-pill-human-fg" },
  blocked: { label: "Blocked", cls: "text-pill-blocked-fg" },
};

/**
 * Agent picker popover (DESIGN.md §4): search row, 40px options with
 * "model · specialty" and availability. Assigning never starts a run (§6.6).
 */
export function AgentPicker({
  agents,
  value,
  onChange,
  children,
  open,
  onOpenChange,
}: {
  agents: Schemas["Agent"][];
  value?: string | null;
  onChange: (agentId: string | null) => void;
  children: ReactNode;
  open?: boolean;
  onOpenChange?: (o: boolean) => void;
}) {
  const [q, setQ] = useState("");
  const [active, setActive] = useState(0);
  // Uncontrolled unless the caller passes `open`.
  const [ownOpen, setOwnOpen] = useState(false);
  const isOpen = open ?? ownOpen;
  const setOpen = (o: boolean) => {
    setOwnOpen(o);
    onOpenChange?.(o);
    if (!o) {
      setQ("");
      setActive(0);
    }
  };
  const shown = useMemo(() => {
    const s = q.trim().toLowerCase();
    return s ? agents.filter((a) => `${a.name} ${a.handle} ${a.specialty}`.toLowerCase().includes(s)) : agents;
  }, [agents, q]);
  const options: Array<Schemas["Agent"] | null> = [...shown, ...(value ? [null] : [])];

  function pick(i: number) {
    const a = options[i];
    if (a === undefined) return;
    onChange(a ? a.id : null);
    setOpen(false);
  }

  return (
    <Popover.Root open={isOpen} onOpenChange={setOpen}>
      <Popover.Trigger asChild>{children}</Popover.Trigger>
      <Popover.Portal>
        <Popover.Content align="start" sideOffset={6} className={cx(floatPanel, "w-[300px] p-0")}>
          <div className="flex items-center gap-2 border-b border-border-subtle px-3">
            <Icon name="search" size={14} className="text-fg-tertiary" />
            <input
              autoFocus
              role="combobox"
              aria-expanded="true"
              aria-controls="agent-options"
              aria-activedescendant={options.length ? `agent-opt-${active}` : undefined}
              aria-label="Assign to agent"
              placeholder="Assign to agent…"
              value={q}
              onChange={(e) => { setQ(e.target.value); setActive(0); }}
              onKeyDown={(e) => {
                if (e.key === "ArrowDown") { e.preventDefault(); setActive((i) => Math.min(i + 1, options.length - 1)); }
                if (e.key === "ArrowUp") { e.preventDefault(); setActive((i) => Math.max(i - 1, 0)); }
                if (e.key === "Enter") { e.preventDefault(); pick(active); }
              }}
              className="h-10 flex-1 bg-transparent text-[13px] outline-none placeholder:text-fg-tertiary"
            />
          </div>
          <div id="agent-options" role="listbox" aria-label="Agents" className="max-h-[320px] overflow-y-auto p-1">
            {options.length === 0 && <div className="px-2 py-2 text-fg-tertiary">No agents match “{q}”</div>}
            {options.map((a, i) =>
              a ? (
                <div
                  key={a.id}
                  id={`agent-opt-${i}`}
                  role="option"
                  aria-selected={a.id === value}
                  onMouseEnter={() => setActive(i)}
                  onClick={() => pick(i)}
                  className={cx("flex h-10 cursor-default items-center gap-2.5 rounded-md px-2", i === active && "bg-hover")}
                >
                  <AgentAvatar name={a.name} colorIndex={a.color_index} size="md" label={false} />
                  <span className="flex min-w-0 flex-1 flex-col leading-tight">
                    <span className="font-medium">{a.name}</span>
                    <span className="truncate text-[11.5px] text-fg-tertiary">
                      {a.model} · {a.specialty}
                    </span>
                  </span>
                  {a.id === value ? (
                    <Icon name="check" size={14} className="text-accent" />
                  ) : (
                    <span className={cx("text-[11.5px]", availability[a.status].cls)}>{availability[a.status].label}</span>
                  )}
                </div>
              ) : (
                <div
                  key="unassign"
                  id={`agent-opt-${i}`}
                  role="option"
                  aria-selected={false}
                  onMouseEnter={() => setActive(i)}
                  onClick={() => pick(i)}
                  className={cx("mt-1 flex h-9 cursor-default items-center gap-2.5 rounded-md border-t border-border-subtle px-2 text-fg-secondary", i === active && "bg-hover")}
                >
                  <UnassignedAvatar />
                  Unassign
                </div>
              ),
            )}
          </div>
          <p className="border-t border-border-subtle px-3 py-2 text-[11.5px] text-fg-tertiary">Assigning queues the ticket. A person dispatches it to start a run.</p>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}

/** Multi-select labels with checkboxes. Applies on each toggle. */
export function LabelPicker({
  labels,
  value,
  onChange,
  children,
}: {
  labels: Schemas["Label"][];
  value: string[];
  onChange: (ids: string[]) => void;
  children: ReactNode;
}) {
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>{children}</DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align="start" sideOffset={6} className={cx(floatPanel, "max-h-[360px] w-52 overflow-y-auto")}>
          {labels.map((l) => {
            const on = value.includes(l.id);
            return (
              <DropdownMenu.CheckboxItem
                key={l.id}
                checked={on}
                className={item}
                onSelect={(e) => e.preventDefault()}
                onCheckedChange={(c) => onChange(c ? [...value, l.id] : value.filter((x) => x !== l.id))}
              >
                <span aria-hidden className="size-2 rounded-full" style={{ background: l.color }} />
                {l.name}
                <Check on={on} />
              </DropdownMenu.CheckboxItem>
            );
          })}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}
