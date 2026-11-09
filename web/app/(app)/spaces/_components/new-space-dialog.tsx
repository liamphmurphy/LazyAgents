"use client";

import { Dialog } from "radix-ui";
import { useRouter } from "next/navigation";
import { useState, type FormEvent, type ReactNode } from "react";
import { useCreateSpace } from "@/lib/api/space-hooks";
import type { SpaceColor } from "@/lib/spaces";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { ColorPicker, ConnectorSelect, MemoryChoice, PersonaSelect, ProjectSelect } from "./space-parts";

const errorText = (e: unknown) => (e instanceof Error ? e.message : "Something went wrong. Please try again.");
const MAX_INSTRUCTIONS = 4000;
const input =
  "h-10 w-full rounded-lg border border-border-strong bg-surface-sunken px-3.5 text-sm text-fg placeholder:text-fg-tertiary focus:border-accent focus:outline-none";

const Label = ({ children, htmlFor }: { children: ReactNode; htmlFor?: string }) => (
  <label htmlFor={htmlFor} className="text-xs text-fg-secondary">
    {children}
  </label>
);

/** Mockup 3: name and color, description, instructions, defaults for new chats and memory. Sources come after. */
export function NewSpaceDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const router = useRouter();
  const create = useCreateSpace();
  const [name, setName] = useState("");
  const [color, setColor] = useState<SpaceColor>("green");
  const [description, setDescription] = useState("");
  const [instructions, setInstructions] = useState("");
  const [persona, setPersona] = useState("");
  const [project, setProject] = useState("");
  const [connectors, setConnectors] = useState<string[]>([]);
  const [spaceMemory, setSpaceMemory] = useState(true);
  const reset = () => {
    setName("");
    setColor("green");
    setDescription("");
    setInstructions("");
    setPersona("");
    setProject("");
    setConnectors([]);
    setSpaceMemory(true);
    create.reset();
  };
  function submit(e: FormEvent) {
    e.preventDefault();
    if (!name.trim() || create.isPending) return;
    create.mutate(
      {
        name: name.trim(),
        color,
        description: description.trim(),
        instructions: instructions.trim(),
        ...(persona ? { persona_id: persona } : {}),
        ...(project ? { project } : {}),
        connector_ids: connectors,
        space_memory: spaceMemory,
      },
      {
        onSuccess: (sp) => {
          onOpenChange(false);
          reset();
          router.push(`/spaces/${sp.id}`);
        },
      },
    );
  }
  const locked = create.isPending;
  return (
    <Dialog.Root
      open={open}
      onOpenChange={(o) => {
        onOpenChange(o);
        if (!o) reset();
      }}
    >
      <Dialog.Portal>
        <Dialog.Overlay className="motion-scrim fixed inset-0 z-40 bg-scrim" />
        <Dialog.Content
          aria-describedby={undefined}
          className="motion-dialog fixed left-1/2 top-[6vh] z-50 flex max-h-[88vh] w-[min(720px,calc(100%-32px))] -translate-x-1/2 flex-col overflow-y-auto rounded-2xl border border-border-float bg-surface-raised px-7 pb-5 pt-6 shadow-[var(--shadow-float)] outline-none"
        >
          <form onSubmit={submit} className="flex flex-col">
            <div className="flex items-center justify-between">
              <Dialog.Title className="text-lg font-semibold text-fg">New space</Dialog.Title>
              <Dialog.Close className="grid size-7 place-items-center rounded-md text-fg-secondary hover:bg-hover hover:text-fg" aria-label="Close">
                <Icon name="x" size={14} strokeWidth={1.6} />
              </Dialog.Close>
            </div>

            <div className="mt-5 flex flex-col gap-2.5">
              <Label htmlFor="space-name">Name</Label>
              <div className="flex gap-3">
                <ColorPicker name={name} value={color} onChange={setColor} disabled={locked} />
                <input id="space-name" className={input} value={name} maxLength={80} autoFocus placeholder="Health" disabled={locked} onChange={(e) => setName(e.target.value)} />
              </div>
            </div>

            <div className="mt-5 flex flex-col gap-2.5">
              <Label htmlFor="space-description">Description</Label>
              <input id="space-description" className={input} value={description} maxLength={300} placeholder="Workouts, nutrition and recovery" disabled={locked} onChange={(e) => setDescription(e.target.value)} />
            </div>

            <div className="mt-5 flex flex-col gap-2.5">
              <div className="flex items-baseline justify-between gap-3">
                <Label htmlFor="space-instructions">Instructions</Label>
                <span className="text-[11px] text-fg-tertiary">Added to every chat in this space, below the persona</span>
              </div>
              <div className="relative">
                <textarea
                  id="space-instructions"
                  value={instructions}
                  maxLength={MAX_INSTRUCTIONS}
                  disabled={locked}
                  placeholder={"Act as my training and nutrition coach.\nPrefer short answers with numbers."}
                  onChange={(e) => setInstructions(e.target.value)}
                  className="h-[132px] w-full resize-none rounded-lg border border-border-strong bg-surface-sunken px-3.5 py-3 text-[13.5px] leading-5 text-fg placeholder:text-fg-tertiary focus:border-accent focus:outline-none"
                />
                <span className="pointer-events-none absolute bottom-2 right-2.5 text-[11px] text-fg-tertiary">
                  {instructions.length} / {MAX_INSTRUCTIONS}
                </span>
              </div>
            </div>

            <div className="mt-6 flex flex-col gap-2.5">
              <span className="text-xs text-fg-secondary">Defaults for new chats</span>
              <div className="flex gap-3 max-sm:flex-col">
                <PersonaSelect value={persona} onChange={setPersona} disabled={locked} />
                <ProjectSelect value={project} onChange={setProject} disabled={locked} />
                <ConnectorSelect value={connectors} onChange={setConnectors} disabled={locked} />
              </div>
            </div>

            <div className="mt-6 flex flex-col gap-2.5">
              <span className="text-xs text-fg-secondary">Memory</span>
              <MemoryChoice value={spaceMemory} onChange={setSpaceMemory} disabled={locked} />
            </div>

            <p className="mt-5 text-xs text-fg-tertiary">Add files, text and websites after the space is created.</p>
            {create.isError && (
              <p role="alert" className="mt-2 text-xs text-pill-blocked-fg">
                {errorText(create.error)}
              </p>
            )}
            <div className="mt-4 flex justify-end gap-2">
              <Dialog.Close asChild>
                <Button variant="secondary" className="!h-9 !px-5">Cancel</Button>
              </Dialog.Close>
              <Button type="submit" variant="primary" className="!h-9 !px-4" disabled={locked || !name.trim()}>
                {locked ? "Creating…" : "Create space"}
              </Button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
