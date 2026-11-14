"use client";

import { Dialog } from "radix-ui";
import { useRouter } from "next/navigation";
import { useState, type FormEvent, type ReactNode } from "react";
import { useCreateSpace } from "@/lib/api/space-hooks";
import { suggestWorkKey, WORK_KEY, type SpaceColor } from "@/lib/spaces";
import { cx } from "@/lib/cx";
import { checkRepoURL } from "@/lib/repo-url";
import { Toggle } from "@/components/ui/controls";
import { Button } from "@/components/ui/button";
import { Icon } from "@/components/ui/icon";
import { ColorPicker, ConnectorSelect, MemoryChoice, PersonaSelect, ProjectSelect } from "./space-parts";

const errorText = (e: unknown) => (e instanceof Error ? e.message : "Something went wrong. Please try again.");
const MAX_INSTRUCTIONS = 4000;
const input =
  "h-[40px] w-full rounded-lg border border-border-strong bg-surface-sunken px-[14px] text-14 text-fg placeholder:text-fg-tertiary focus:border-accent focus:outline-none";

const Label = ({ children, htmlFor }: { children: ReactNode; htmlFor?: string }) => (
  <label htmlFor={htmlFor} className="text-12 text-fg-secondary">
    {children}
  </label>
);

/** Mockup 3: name and color, description, instructions, defaults for new chats, work (LAZYAGENTS-213 frame C) and memory. Sources come after. */
export function NewSpaceDialog({ open, onOpenChange, defaultTracking = false }: { open: boolean; onOpenChange: (open: boolean) => void; defaultTracking?: boolean }) {
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
  const [tracking, setTracking] = useState(defaultTracking);
  // The key follows the name until it's typed into.
  const [typedKey, setTypedKey] = useState<string | null>(null);
  const [repo, setRepo] = useState("");
  const key = (typedKey ?? suggestWorkKey(name)).trim();
  const repoCheck = checkRepoURL(repo);
  const workOk = !tracking || (WORK_KEY.test(key) && repoCheck.state !== "invalid");
  const reset = () => {
    setName("");
    setColor("green");
    setDescription("");
    setInstructions("");
    setPersona("");
    setProject("");
    setConnectors([]);
    setSpaceMemory(true);
    setTracking(defaultTracking);
    setTypedKey(null);
    setRepo("");
    create.reset();
  };
  function submit(e: FormEvent) {
    e.preventDefault();
    if (!name.trim() || create.isPending || !workOk) return;
    create.mutate(
      {
        name: name.trim(),
        color,
        description: description.trim(),
        instructions: instructions.trim(),
        ...(persona ? { persona_id: persona } : {}),
        ...(tracking ? { work: { key, ...(repo.trim() ? { repo_url: repo.trim() } : {}) } } : project ? { project } : {}),
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
          className="motion-dialog fixed left-1/2 top-[6vh] z-50 flex max-h-[88vh] w-[min(720px,calc(100%-32px))] -translate-x-1/2 flex-col overflow-y-auto rounded-2xl border border-border-float bg-surface-raised px-[28px] pb-[20px] pt-[24px] shadow-[var(--shadow-float)] outline-none"
        >
          <form onSubmit={submit} className="flex flex-col">
            <div className="flex items-center justify-between">
              <Dialog.Title className="text-18 font-semibold text-fg">New space</Dialog.Title>
              <Dialog.Close className="grid size-[28px] place-items-center rounded-md text-fg-secondary hover:bg-hover hover:text-fg" aria-label="Close">
                <Icon name="x" size={14} strokeWidth={1.6} />
              </Dialog.Close>
            </div>

            <div className="mt-[20px] flex flex-col gap-[10px]">
              <Label htmlFor="space-name">Name</Label>
              <div className="flex gap-[12px]">
                <ColorPicker name={name} value={color} onChange={setColor} disabled={locked} />
                <input id="space-name" className={input} value={name} maxLength={80} autoFocus placeholder="Health" disabled={locked} onChange={(e) => setName(e.target.value)} />
              </div>
            </div>

            <div className="mt-[20px] flex flex-col gap-[10px]">
              <Label htmlFor="space-description">Description</Label>
              <input id="space-description" className={input} value={description} maxLength={300} placeholder="Workouts, nutrition and recovery" disabled={locked} onChange={(e) => setDescription(e.target.value)} />
            </div>

            <div className="mt-[20px] flex flex-col gap-[10px]">
              <div className="flex items-baseline justify-between gap-[12px]">
                <Label htmlFor="space-instructions">Instructions</Label>
                <span className="text-11 text-fg-tertiary">Added to every chat in this space, below the persona</span>
              </div>
              <div className="relative">
                <textarea
                  id="space-instructions"
                  value={instructions}
                  maxLength={MAX_INSTRUCTIONS}
                  disabled={locked}
                  placeholder={"Act as my training and nutrition coach.\nPrefer short answers with numbers."}
                  onChange={(e) => setInstructions(e.target.value)}
                  className="h-[132px] w-full resize-none rounded-lg border border-border-strong bg-surface-sunken px-[14px] py-[12px] text-14 leading-[20px] text-fg placeholder:text-fg-tertiary focus:border-accent focus:outline-none"
                />
                <span className="pointer-events-none absolute bottom-[8px] right-[10px] text-11 text-fg-tertiary">
                  {instructions.length} / {MAX_INSTRUCTIONS}
                </span>
              </div>
            </div>

            <div className="mt-[24px] flex flex-col gap-[10px]">
              <span className="text-12 text-fg-secondary">Defaults for new chats</span>
              <div className="flex gap-[12px] max-sm:flex-col">
                <PersonaSelect value={persona} onChange={setPersona} disabled={locked} />
                {!tracking && <ProjectSelect value={project} onChange={setProject} disabled={locked} />}
                <ConnectorSelect value={connectors} onChange={setConnectors} disabled={locked} />
              </div>
            </div>

            <div className="mt-[24px] flex flex-col gap-[10px]">
              <Toggle
                checked={tracking}
                disabled={locked}
                onChange={setTracking}
                label={
                  <span className="text-13 text-fg">
                    Track work <span className="text-12 text-fg-tertiary">tickets, epics, runs and a repository</span>
                  </span>
                }
              />
              {tracking && (
                <div className="flex gap-[12px] max-sm:flex-col">
                  <label className="flex w-[150px] flex-col gap-[6px] text-12 text-fg-secondary max-sm:w-full">
                    Key
                    <input aria-label="Key" className={cx(input, "font-mono uppercase")} value={key} maxLength={10} placeholder="KEY" disabled={locked} onChange={(e) => setTypedKey(e.target.value.toUpperCase())} />
                  </label>
                  <label className="flex flex-1 flex-col gap-[6px] text-12 text-fg-secondary">
                    Repository (optional)
                    <input aria-label="Repository" className={input} value={repo} placeholder="https://github.com/owner/repo" disabled={locked} onChange={(e) => setRepo(e.target.value)} />
                  </label>
                </div>
              )}
              {tracking && key && !WORK_KEY.test(key) && <p className="text-12 text-pill-blocked-fg">Use 2–10 letters or digits, starting with a letter.</p>}
              {tracking && repoCheck.state === "invalid" && <p className="text-12 text-pill-blocked-fg">{repoCheck.message}</p>}
            </div>

            <div className="mt-[24px] flex flex-col gap-[10px]">
              <span className="text-12 text-fg-secondary">Memory</span>
              <MemoryChoice value={spaceMemory} onChange={setSpaceMemory} disabled={locked} />
            </div>

            <p className="mt-[20px] text-12 text-fg-tertiary">Add files, text and websites after the space is created.</p>
            {create.isError && (
              <p role="alert" className="mt-[8px] text-12 text-pill-blocked-fg">
                {errorText(create.error)}
              </p>
            )}
            <div className="mt-[16px] flex justify-end gap-[8px]">
              <Dialog.Close asChild>
                <Button variant="secondary" className="!h-[36px] !px-[20px]">Cancel</Button>
              </Dialog.Close>
              <Button type="submit" variant="primary" className="!h-[36px] !px-[16px]" disabled={locked || !name.trim() || !workOk}>
                {locked ? "Creating…" : "Create space"}
              </Button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
