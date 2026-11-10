"use client";

import { useCanvasPanel } from "@/lib/use-canvas-panel";
import { composerFrame, composerInput, ResearchToggle, TemporaryToggle } from "./reply-tools";
import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { useBootstrap, useChat, useChats, useStartChat } from "@/lib/api/hooks";
import { usePersonas } from "@/lib/api/persona-hooks";
import { newestChat } from "@/lib/chat-ui";
import { codesInRepos } from "@/lib/persona-ui";
import { parseAttach, removeChip, toContextRefs, type Chip } from "@/lib/chat-context";
import { Button } from "@/components/ui/button";
import { Banner, Skeleton, TextArea } from "@/components/ui/primitives";
import { NoHarnessNote, useHarnessHealth } from "@/components/domain/setup";
import { Conversation } from "./conversation";
import { AttachButton, AttachmentChips, DropOverlay, useAttachments, useFileDrop } from "./attachments";
import { ChipList } from "./context-chips";
import { useNewConversation } from "./chat-history";
import { CanvasButton, CanvasFullscreen } from "@/components/domain/canvas-view";
import { PersonaAvatar, PersonaSwitcher, RepoSelect, personaLook, useOpenPersonaManager } from "./persona";
import { CommitEmailPicker } from "./commit-email";
import { needsCommitEmailPick } from "@/lib/commit-email";

const errorText = (e: unknown) => (e instanceof Error ? e.message : "Something went wrong. Please try again.");

const SUGGESTIONS = [
  "What did my agents finish this week?",
  "What’s blocking my open epics?",
  "Draft tickets from a spec",
  "Search the web for the latest on a topic",
];

/** A first-time conversation (mockup frame 2): greeting, suggestions, and a composer that starts the chat. */
function Welcome({ disabled }: { disabled: boolean }) {
  const router = useRouter();
  const search = useSearchParams();
  const boot = useBootstrap();
  const start = useStartChat();
  const personas = usePersonas();
  // Undefined until picked: the chat gets the persona the link named (the landing page's most used), else the default.
  const [picked, setPicked] = useState<string>();
  const linked = search.get("persona");
  const linkedKnown = personas.data?.personas.some((p) => p.id === linked) ? linked! : undefined;
  const personaId = picked ?? linkedKnown ?? personas.data?.default_persona_id;
  const fullPersona = personas.data?.personas.find((p) => p.id === personaId);
  const persona = personaLook(fullPersona);
  // A coding persona works in one of its repos: the first unless another is picked.
  const [repoPicked, setRepoPicked] = useState<string>();
  const repos = fullPersona?.repos ?? [];
  const repo = codesInRepos(fullPersona) ? (repoPicked && repos.includes(repoPicked) ? repoPicked : repos[0]) : undefined;
  // With several addresses, a coding chat's first message waits for which one its commits use.
  const emails = boot.data?.me.emails ?? [];
  const [commitEmail, setCommitEmail] = useState<string>();
  const askCommitEmail = needsCommitEmailPick(emails, repo, commitEmail);
  const managePersonas = useOpenPersonaManager();
  const [text, setText] = useState("");
  const [research, setResearch] = useState(false);
  const [temporary, setTemporary] = useState(false);
  // A "Start chat" link with no chat yet (?attach=…) lands here; its chips go with the first message.
  const [chips, setChips] = useState<Chip[]>(() => parseAttach(search.getAll("attach"), search.get("attach_title")));
  const files = useAttachments();
  const area = useRef<HTMLTextAreaElement>(null);
  const name = boot.data?.me.name?.split(" ")[0];
  const locked = disabled || start.isPending;
  const drop = useFileDrop(files.add, !locked);

  function submit() {
    const body = text.trim();
    if (!body || locked || files.pending || askCommitEmail) return;
    const context_refs = toContextRefs(chips);
    start.mutate(
      { persona_id: personaId, ...(temporary ? { temporary } : {}), ...(repo ? { repo_url: repo, ...(commitEmail ? { commit_email: commitEmail } : {}) } : {}), message: { body, ...(research ? { research: true } : {}), ...(context_refs ? { context_refs } : {}), ...(files.ids.length ? { attachment_ids: files.ids } : {}) } },
      { onSuccess: (c) => router.push(`/chat/${c.id}`) },
    );
  }
  return (
    <div className="relative flex min-h-0 flex-1 flex-col" {...drop.props}>
      <DropOverlay over={drop.over} />
      <div className="flex flex-1 flex-col items-center justify-center gap-3 overflow-y-auto px-6 py-8 text-center">
        <PersonaAvatar persona={persona} size={48} />
        <h1 className="text-xl font-semibold text-fg">{name ? `Hi ${name}, what can I help with?` : "What can I help with?"}</h1>
        <p className="max-w-[520px] text-[13px] text-fg-secondary">
          I can read your runs, tickets, epics, wiki and files, search the web, and create tickets for you. I remember what matters and tell you when I do.
        </p>
        <ul aria-label="Suggestions" className="mt-2 grid w-full max-w-[520px] grid-cols-2 gap-2 max-sm:grid-cols-1">
          {SUGGESTIONS.map((s) => (
            <li key={s}>
              <button
                type="button"
                disabled={locked}
                onClick={() => {
                  setText(s);
                  area.current?.focus();
                }}
                className="h-full w-full rounded-xl border border-border bg-surface px-3.5 py-3 text-left text-[13px] text-fg hover:bg-hover disabled:opacity-50"
              >
                {s}
              </button>
            </li>
          ))}
        </ul>
        <p className="mt-2 text-[11.5px] text-fg-tertiary">Your memory is private to you and separate from the team wiki.</p>
      </div>
      <form
        className="mx-auto flex w-full max-w-[860px] flex-col gap-2 px-6 pb-5"
        onPaste={files.onPaste}
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
      >
        {start.isError && (
          <p role="alert" className="text-[12.5px] text-pill-blocked-fg">{errorText(start.error)}</p>
        )}
        {fullPersona?.capabilities.code && !repos.length && (
          <p className="text-[12.5px] text-fg-secondary" data-testid="no-repos-hint">
            {fullPersona.name} can write code but has no repository yet.{" "}
            <button type="button" className="text-accent hover:underline" onClick={managePersonas}>
              Edit the persona to add one
            </button>
          </p>
        )}
        {repo && emails.length > 1 && (
          <CommitEmailPicker emails={emails} name={boot.data?.me.name ?? ""} repo={repo} value={commitEmail} onPick={setCommitEmail} disabled={locked} required />
        )}
        <AttachmentChips attachments={files} />
        <ChipList chips={chips} onRemove={(k) => setChips((cs) => removeChip(cs, k))} disabled={locked} />
        <div className={composerFrame}>
        <TextArea
          ref={area}
          aria-label="Message"
          className={composerInput}
          rows={3}
          value={text}
          disabled={locked}
          placeholder={`Message ${persona.id ? persona.name : "your assistant"}… Enter sends, Shift+Enter adds a line.`}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
              e.preventDefault();
              submit();
            }
          }}
        />
        <div className="flex items-end gap-2">
          <div className="flex min-w-0 flex-1 flex-wrap items-center gap-2">
            <PersonaSwitcher value={personaId} onChange={setPicked} disabled={locked} />
            {repo && <RepoSelect repos={repos} value={repo} onChange={setRepoPicked} disabled={locked} />}
            <AttachButton onFiles={files.add} disabled={locked} compact />
            <ResearchToggle on={research} onChange={setResearch} disabled={locked} />
            <TemporaryToggle on={temporary} onChange={setTemporary} disabled={locked} />
          </div>
          <Button
            className="flex-none"
            type="submit"
            variant="primary"
            disabled={locked || files.pending || !text.trim() || askCommitEmail}
            title={askCommitEmail ? "Pick which email commits use first" : undefined}
          >
            {start.isPending ? "Starting…" : research ? "Plan it" : "Send"}
          </Button>
        </div>
        </div>
      </form>
    </div>
  );
}

/**
 * The Personal Assistant (replaces Chat): one conversation surface with no agent
 * picker; its history (rename, archive, restore, delete) is in the main menu's Chat mode.
 * /chat opens the newest conversation, or the first-time welcome; /chat?new (the landing
 * page, with ?persona=) always opens the welcome; "New
 * conversation" starts afresh. Memory and Personas open as right-hand panels from the main menu.
 */
export function ChatScreen({ id }: { id?: string }) {
  const router = useRouter();
  const search = useSearchParams();
  const current = useChat(id);
  const canvas = useCanvasPanel();
  const turnRunning = current.data?.state === "running";
  const health = useHarnessHealth();
  const noHarness = !!health && !health.ready;
  // /chat resumes the newest conversation, unless a "Start chat" link brought context to attach
  // or the landing page asked for a new one (?new).
  const fresh = search.has("attach") || search.has("new");
  // Resume the newest conversation with any persona, whatever the sidebar shows.
  const chats = useChats(false, "all");
  const newest = newestChat(chats.data?.chats);
  useEffect(() => {
    if (id || fresh || !newest) return;
    const q = new URLSearchParams(search.toString());
    q.delete("project"); // Old links; a chat carries its own project.
    const qs = q.toString();
    router.replace(qs ? `/chat/${newest.id}?${qs}` : `/chat/${newest.id}`);
  }, [id, fresh, newest, router, search]);

  const conversation = useNewConversation(id);
  const resolving = !id && !fresh && (!chats.data || !!newest);
  return (
    <div className="flex h-dvh flex-col">
      <div className="relative flex min-h-0 flex-1 max-md:flex-col">
          <div className="flex min-w-0 flex-1 flex-col">
            {(noHarness || conversation.error) && (
              <div className="flex flex-col gap-1 px-6 pt-3">
                {noHarness && <NoHarnessNote health={health} />}
                {conversation.error && <p role="alert" className="text-xs text-pill-blocked-fg">{conversation.error}</p>}
              </div>
            )}
            {id ? (
              <Conversation key={id} id={id} onNew={conversation.start} actions={<CanvasButton chatId={id} live={turnRunning} />} />
            ) : (
              <>
                {chats.isError ? (
                  <div className="p-6">
                    <Banner tone="blocked" title="Couldn’t load your conversations">{errorText(chats.error)}</Banner>
                  </div>
                ) : resolving ? (
                  <div aria-busy="true" className="flex flex-1 flex-col gap-4 p-6">
                    <Skeleton className="h-6 w-1/3" />
                    <Skeleton className="h-16 w-2/3" />
                  </div>
                ) : (
                  <Welcome disabled={noHarness} />
                )}
              </>
            )}
          </div>
          {id && canvas.fullscreen ? <CanvasFullscreen chatId={id} live={turnRunning} readOnly={!!current.data?.archived} agent={current.data?.persona?.name ?? "Assistant"} /> : null}
      </div>
    </div>
  );
}
