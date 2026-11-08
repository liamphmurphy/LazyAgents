import test from "node:test";
import assert from "node:assert/strict";
import { activityOf, bareTool, chatStatus, chatCalls, chatStatusOf, chatSteps, stepDetail, chatTitle, foldSteps, interimLine, linkLabel, needsSwitchConfirm, replyCost, runningSince, toolCalls, turnWindows } from "./chat-ui.ts";

test("tool calls pair with results and skip events before the turn", () => {
  const events = [
    { seq: 1, type: "tool_call", at: "2026-11-06T10:00:00Z", payload: { id: "old", tool: "whoami" } },
    { seq: 2, type: "tool_call", at: "2026-11-06T10:01:00Z", payload: { id: "x", tool: "read_page", input: { ref: "Plan" } } },
    { seq: 3, type: "tool_result", at: "2026-11-06T10:01:01Z", payload: { id: "x" } },
    { seq: 4, type: "tool_call", at: "2026-11-06T10:01:02Z", payload: { id: "y", tool: "get_ticket", input: { ticket: "WEB-3" } } },
  ];
  assert.deepEqual(toolCalls(events, "2026-11-06T10:00:30Z"), [
    { key: 2, tool: "read_page", input: { ref: "Plan" }, done: true },
    { key: 4, tool: "get_ticket", input: { ticket: "WEB-3" }, done: false },
  ]);
  assert.equal(toolCalls(undefined).length, 0);
  // `until` (the next user message) closes the turn.
  assert.deepEqual(toolCalls(events, "2026-11-06T10:00:30Z", "2026-11-06T10:01:01.500Z").map((c) => c.key), [2]);
});

test("MCP tool names lose the lazyagents prefix from either harness", () => {
  assert.equal(bareTool("mcp__lazyagents__read_page"), "read_page");
  assert.equal(bareTool("lazyagents.read_page"), "read_page");
  assert.equal(bareTool("lazyagents_read_page"), "read_page");
  assert.equal(bareTool("read_page"), "read_page");
  assert.equal(bareTool("mcp__github__get_issue"), "github.get_issue");
});

test("the live line names the page, ticket or query being used", () => {
  assert.deepEqual(activityOf({ tool: "mcp__lazyagents__read_page", input: { ref: "Architecture/ADR - Project chat sessions" } }), {
    verb: "Reading", ref: "Architecture/ADR - Project chat sessions", label: "ADR - Project chat sessions",
  });
  assert.deepEqual(activityOf({ tool: "read_page", input: { ref: "[[ADR-012]]" } }), { verb: "Reading", ref: "ADR-012", label: "ADR-012" });
  assert.deepEqual(activityOf({ tool: "read_artifact", input: { artifact: "ADR-012", section: "risks" } }), {
    verb: "Reading", ref: "ADR-012#risks", label: "ADR-012 › risks",
  });
  assert.deepEqual(activityOf({ tool: "read_artifact", input: { artifact: "01a1b4ab-1853-7647-8534-85c284cec5ec" } }), { verb: "Reading an artifact" });
  assert.deepEqual(activityOf({ tool: "get_ticket", input: { ticket: "LAZYAGENTS-12" } }), { verb: "Opening", ref: "LAZYAGENTS-12", label: "LAZYAGENTS-12" });
  assert.deepEqual(activityOf({ tool: "search_knowledge", input: { q: "harness switch" } }), { verb: "Searching", text: "“harness switch”" });
  assert.deepEqual(activityOf({ tool: "read_page", input: {} }), { verb: "Reading a page" });
  assert.deepEqual(activityOf({ tool: "shell", input: { command: "ls" } }), { verb: "Running a command" });
  assert.deepEqual(activityOf({ tool: "mcp__github__get_issue", input: null }), { verb: "Using", text: "github.get_issue" });
});

test("link labels drop folders and keep the section", () => {
  assert.equal(linkLabel("Eng/Ledger/Runbook"), "Runbook");
  assert.equal(linkLabel("Runbook#rollback"), "Runbook › rollback");
  assert.equal(linkLabel("#rollback"), "rollback");
});

test("only a harness change on a chat with messages asks first", () => {
  const claude = { harness: "claude-code" };
  const codex = { harness: "codex" };
  assert.equal(needsSwitchConfirm(claude, codex, null, 3), true);
  assert.equal(needsSwitchConfirm(claude, { harness: "claude-code" }, null, 3), false, "same harness, new model");
  assert.equal(needsSwitchConfirm(claude, codex, null, 0), false, "nothing to lose yet");
  assert.equal(needsSwitchConfirm(claude, codex, codex, 3), false, "already confirmed for codex");
  assert.equal(needsSwitchConfirm(claude, { harness: "other" }, codex, 3), true);
});

test("reply cost keeps small turns visible and never shows unknown as free", () => {
  assert.equal(replyCost(0.0421), "$0.042");
  assert.equal(replyCost(1.5), "$1.50");
  assert.equal(replyCost(0.0002), "< $0.001");
  assert.equal(replyCost(0), undefined);
  assert.equal(replyCost(undefined), undefined);
  assert.equal(replyCost(Number.NaN), undefined);
});

test("an empty chat title reads as New conversation", () => {
  assert.equal(chatTitle({ title: "" }), "New conversation");
  assert.equal(chatTitle({ title: "   " }), "New conversation");
  assert.equal(chatTitle({ title: "Roadmap" }), "Roadmap");
});

test("the chat status is high level and never leaks the call's details", () => {
  const cases = [
    [{ tool: "mcp__lazyagents__read_page", input: { ref: "Architecture/ADR - Secret" } }, "looking through the wiki"],
    [{ tool: "search_knowledge", input: { q: "payroll" } }, "looking through the wiki"],
    [{ tool: "create_ticket", input: { title: "x" } }, "drafting a ticket"],
    [{ tool: "save_memory", input: { body: "likes tea" } }, "making a note"],
    [{ tool: "WebFetch", input: { url: "https://example.com/private" } }, "searching the web"],
    [{ tool: "image_gen", input: {} }, "creating an image"],
    [{ tool: "mcp__lazyagents__generate_image", input: { prompt: "a cat" } }, "creating an image"],
    [{ tool: "Grep", input: { pattern: "TODO" } }, "reading the code"],
    [{ tool: "apply_patch", input: { files: ["modify src/a.ts"] } }, "writing code"],
    [{ tool: "Bash", input: { command: "go test ./internal/..." } }, "running tests"],
    [{ tool: "shell", input: { command: "bash -lc 'npm run test:unit'" } }, "running tests"],
    [{ tool: "shell", input: { command: "git commit -m 'secret plan'" } }, "committing changes"],
    [{ tool: "Bash", input: { command: "git push -u origin vesper/ava-1234abcd" } }, "pushing its branch"],
    [{ tool: "Bash", input: { command: "gh pr create --fill" } }, "preparing a pull request"],
    [{ tool: "Bash", input: { command: "npm ci" } }, "installing dependencies"],
    [{ tool: "Bash", input: { command: "go vet ./..." } }, "checking the build"],
    [{ tool: "Bash", input: { command: "git status" } }, "looking over the repository"],
    [{ tool: "Bash", input: { command: "rg -n token src" } }, "reading the code"],
    [{ tool: "exec_command", input: { cmd: "cp /h/.codex/generated_images/t/a.png .lazyagents/outbox/a.png" } }, "saving the image"],
    [{ tool: "Read", input: { file_path: "/w/.lazyagents/attachments/generated/turn-1/1-fox.png" } }, "looking at the image"],
    [{ tool: "Read", input: { file_path: "src/main.go" } }, "reading the code"],
    [{ tool: "Bash", input: { command: "python3 scripts/migrate.py" } }, "working in the terminal"],
    [{ tool: "mcp__lazyagents__canvas_patch", input: { body: "secret" } }, "revising a canvas"],
    [{ tool: "canvas_reply", input: { body: "secret" } }, "commenting on a canvas"],
    [{ tool: "propose_workflow_edit", input: { graph: "secret" } }, "sketching a workflow"],
    [{ tool: "get_video_transcript", input: { url: "https://example.com/private" } }, "watching a video"],
    [{ tool: "some.other_tool", input: { secret: "x" } }, "working on it"],
  ];
  for (const [call, want] of cases) assert.equal(chatStatusOf(call).phrase, want, call.tool);
  assert.deepEqual(chatStatus([]), { kind: "thinking", phrase: "thinking" });
  assert.deepEqual(chatStatus([{ tool: "Read", input: {} }, { tool: "Bash", input: { command: "go test ./..." } }]), { kind: "tests", phrase: "running tests" });
});

test("each tool call maps to the kind of scene it shows", () => {
  const bash = (command) => ({ tool: "Bash", input: { command } });
  const cases = [
    [{ tool: "mcp__lazyagents__read_page", input: {} }, "wiki"],
    [{ tool: "get_ticket", input: {} }, "tickets"],
    [{ tool: "create_epic", input: {} }, "tickets"],
    [{ tool: "get_run", input: {} }, "runs"],
    [{ tool: "Read", input: { file_path: "src/a.go" } }, "reading-code"],
    [bash("rg -n token src"), "reading-code"],
    [{ tool: "apply_patch", input: {} }, "writing-code"],
    [bash("go test ./..."), "tests"],
    [bash("npm run lint"), "build"],
    [bash("npm ci"), "install"],
    [{ tool: "WebFetch", input: {} }, "web"],
    [{ tool: "generate_image", input: {} }, "image"],
    [{ tool: "Read", input: { file_path: "/w/.lazyagents/attachments/generated/t/1.png" } }, "image"],
    [{ tool: "get_video_transcript", input: {} }, "video"],
    [{ tool: "canvas_create", input: {} }, "canvas"],
    [{ tool: "canvas_comment", input: {} }, "canvas-comment"],
    [{ tool: "read_workflow", input: {} }, "workflow"],
    [{ tool: "save_memory", input: {} }, "memory"],
    [{ tool: "some.other_tool", input: {} }, "thinking"],
    [{ tool: "", input: undefined }, "thinking"],
    [bash("python3 scripts/migrate.py"), "terminal"],
  ];
  for (const [call, want] of cases) assert.equal(chatStatusOf(call).kind, want, call.tool);
});

test("GitHub work is detected from MCP tools, gh and repository commands, without leaking details", () => {
  const bash = (command) => ({ tool: "Bash", input: { command } });
  const cases = [
    [{ tool: "mcp__github__create_pull_request", input: { title: "secret" } }, "pull-request", "preparing a pull request"],
    [{ tool: "github.update_pull_request", input: {} }, "pull-request", "preparing a pull request"],
    [{ tool: "mcp__github__list_issues", input: { repo: "secret" } }, "github", "working on GitHub"],
    [{ tool: "mcp__plugin_github_github__get_issue", input: {} }, "github", "working on GitHub"],
    [bash("gh issue view 12"), "github", "working on GitHub"],
    [bash("gh run watch"), "github", "working on GitHub"],
    [bash("gh pr create --fill"), "pull-request", "preparing a pull request"],
    [bash("git -C /work/repo push origin main"), "push", "pushing its branch"],
    [bash("git -c user.name=x commit -m secret"), "commit", "committing changes"],
    [bash("git --no-pager log -3"), "repo-history", "looking over the repository"],
    [bash("git add -A"), "repo-history", "looking over the repository"],
    [bash("git stash"), "repo-history", "looking over the repository"],
  ];
  for (const [call, kind, phrase] of cases) assert.deepEqual(chatStatusOf(call), { kind, phrase }, call.tool);
  // Tests win over reading the repository's state in one chained command.
  assert.equal(chatStatusOf(bash("git status && go test ./...")).kind, "tests");
});

test("the opening line shows only once a tool call follows it", () => {
  const at = (s) => `2026-11-06T10:0${s}:00Z`;
  const msg = (seq, text, extra = {}) => ({ seq, type: "message", at: at(seq), payload: { role: "assistant", text, ...extra } });
  const call = (seq) => ({ seq, type: "tool_call", at: at(seq), payload: { id: `c${seq}`, tool: "list_runs" } });
  const since = "2026-11-06T10:00:30Z";
  // A straight answer has no tool call: its text is the reply, not an opening line.
  assert.equal(interimLine([msg(1, "Hello!")], since), undefined);
  assert.equal(interimLine([msg(1, "  Let me check the runs.  "), call(2), msg(3, "Two are running.")], since), "Let me check the runs.");
  // An earlier turn's text and plan messages don't count.
  assert.equal(interimLine([msg(0, "old"), msg(1, "plan", { kind: "plan" }), call(2)], since), undefined);
  assert.equal(interimLine(undefined), undefined);
});

test("each turn's reply gets its own event window, including a follow-up with no user message", () => {
  const t = (m) => `2026-11-06T10:${String(m).padStart(2, "0")}:00Z`;
  const list = [
    { id: "u1", role: "user", created_at: t(0) },
    { id: "a1", role: "assistant", created_at: t(1), run_id: "r", turn_seq: 1 },
    { id: "u2", role: "user", created_at: t(2) },
    // The opening line and the answer of one turn share a window.
    { id: "o2", role: "assistant", created_at: t(3), run_id: "r", turn_seq: 2 },
    { id: "a2", role: "assistant", created_at: t(3), run_id: "r", turn_seq: 2 },
    { id: "s2", role: "system", created_at: t(3), run_id: "r" },
    // The image follow-up: another run turn the person didn't start.
    { id: "f2", role: "assistant", created_at: t(5), run_id: "r", turn_seq: 3 },
  ];
  const w = turnWindows(list);
  assert.deepEqual([...w.keys()], ["a1", "a2", "f2"]);
  assert.deepEqual(w.get("a1"), { since: t(0), until: t(2), followup: false });
  assert.deepEqual(w.get("a2"), { since: t(2), until: t(3), followup: false });
  assert.deepEqual(w.get("f2"), { since: t(3), until: undefined, followup: true });
  // Messages without a turn keep one window per user message, under its last reply.
  const old = [list[0], { id: "x", role: "assistant", created_at: t(1) }, { id: "y", role: "assistant", created_at: t(1) }];
  assert.deepEqual([...turnWindows(old).keys()], ["y"]);
  assert.equal(turnWindows(undefined).size, 0);
});

test("the running turn starts at the newest message", () => {
  const user = { id: "u", role: "user", created_at: "2026-11-06T10:00:00Z" };
  const reply = { id: "a", role: "assistant", created_at: "2026-11-06T10:01:00Z", run_id: "r", turn_seq: 1 };
  const note = { id: "s", role: "system", created_at: "2026-11-06T10:01:00.500Z" };
  assert.equal(runningSince([user]), user.created_at);
  // The image follow-up has no message of its own: it starts after the reply and its notes.
  assert.equal(runningSince([user, reply, note]), note.created_at);
  assert.equal(runningSince(undefined), undefined);
});

const stepsOf = (calls) => chatSteps(calls.map((c, key) => ({ key, done: true, input: {}, ...c })));

test("chatSteps follows the newest whole-list plan and marks steps added after work began", () => {
  const todo = (...rows) => ({ tool: "TodoWrite", input: { todos: rows.map(([content, status]) => ({ content, status })) } });
  const { steps, planned } = stepsOf([todo(["Read", "in_progress"], ["Test", "pending"]), { tool: "Read" }, todo(["Read", "completed"], ["Fix", "in_progress"], ["Test", "pending"])]);
  assert.equal(planned, true);
  assert.deepEqual(steps.map((s) => [s.title, s.state, !!s.added]), [["Read", "done", false], ["Fix", "active", true], ["Test", "planned", false]]);
  assert.deepEqual(stepsOf([{ tool: "todowrite", input: { todos: [{ content: "One", status: "in_progress" }, { content: "Gone", status: "cancelled" }] } }]).steps.map((s) => [s.title, s.state]), [["One", "active"]]);
});

test("chatSteps tracks TaskCreate and TaskUpdate by the id in the result", () => {
  const create = (subject, id) => ({ tool: "TaskCreate", input: { subject }, output: `Task #${id} created successfully: ${subject}` });
  const update = (taskId, status) => ({ tool: "TaskUpdate", input: { taskId, status } });
  const { steps } = stepsOf([create("A", 7), create("B", 8), create("C", 9), update("7", "completed"), update("8", "in_progress"), update("9", "deleted"), create("D", 10)]);
  assert.deepEqual(steps.map((s) => [s.title, s.state, !!s.added]), [["A", "done", false], ["B", "active", false], ["D", "planned", true]]);
});

test("a step keeps its key when another is added before it", () => {
  const todo = (...titles) => ({ tool: "TodoWrite", input: { todos: titles.map((content) => ({ content, status: "pending" })) } });
  const key = (calls, title) => stepsOf(calls).steps.find((s) => s.title === title).key;
  assert.equal(key([todo("A", "B")], "B"), key([todo("A", "New", "B")], "B"));
  assert.notEqual(...stepsOf([todo("Same", "Same")]).steps.map((s) => s.key));
});

test("every harness's tools and each chat tool land on a described step", () => {
  const phrases = (...tools) => stepsOf(tools.map((tool) => ({ tool }))).steps.map((s) => s.title);
  assert.deepEqual(phrases("read", "lazyagents_get_ticket", "edit", "websearch"), ["Reading the code", "Checking tickets", "Writing code", "Searching the web"]);
  assert.deepEqual(phrases("mcp__lazyagents__list_saved_links", "mcp__lazyagents__propose_persona", "mcp__lazyagents__submit_research_report"), ["Checking your saved links", "Sketching a new persona", "Writing up its research"]);
  assert.deepEqual(phrases("mcp__lazyagents__canvas_create", "mcp__lazyagents__canvas_patch", "mcp__lazyagents__canvas_patch"), ["Drafting a canvas", "Revising a canvas"]);
  assert.deepEqual(phrases("mcp__other__mystery"), ["Working on it"]);
  assert.deepEqual(phrases("mcp__lazyagents__whoami", "ToolSearch", "Skill", "mcp__lazyagents__set_chat_title"), []);
});

test("chatSteps groups a planless turn's calls, skipping bookkeeping", () => {
  const { steps, planned } = stepsOf([{ tool: "Read" }, { tool: "ToolSearch" }, { tool: "Grep" }, { tool: "Edit" }, { tool: "Bash", input: { command: "go test ./..." } }]);
  assert.equal(planned, false);
  assert.deepEqual(steps.map((s) => s.state), ["done", "done", "active"]);
  assert.equal(steps[0].title, chatStatusOf({ tool: "Read", input: {} }).phrase.replace(/^./, (c) => c.toUpperCase()));
  assert.deepEqual(chatSteps([]), { steps: [], planned: false });
});

test("foldSteps folds old finished steps and caps upcoming ones", () => {
  const make = (state, n, p) => Array.from({ length: n }, (_, i) => ({ key: `${p}${i}`, title: `${p}${i}`, state }));
  const f = foldSteps([...make("done", 5, "d"), ...make("active", 1, "a"), ...make("planned", 6, "p")]);
  assert.deepEqual([f.earlier, f.later], [3, 2]);
  assert.deepEqual(f.shown.map((s) => s.key), ["d3", "d4", "a0", "p0", "p1", "p2", "p3"]);
  assert.equal(foldSteps(make("done", 3, "d")).earlier, 0);
});

test("rich reply blocks join the calls in order and become steps", () => {
  const events = [
    { seq: 1, type: "tool_call", at: "2026-11-06T10:01:00Z", payload: { id: "a", tool: "Read", input: {} } },
    { seq: 2, type: "lifecycle", at: "2026-11-06T10:01:01Z", payload: { state: "harness_started" } },
    { seq: 3, type: "lifecycle", at: "2026-11-06T10:01:02Z", payload: { state: "reply_block", kind: "app" } },
  ];
  const calls = chatCalls(events, "2026-11-06T10:00:30Z");
  assert.deepEqual(calls.map((c) => c.tool), ["Read", "reply_block"]);
  assert.deepEqual(chatStatus(calls), { kind: "canvas", phrase: "building an interactive app" });
  assert.deepEqual(chatSteps(calls).steps.map((s) => [s.title, s.state]), [["Reading the code", "done"], ["Building an interactive app", "active"]]);
  assert.equal(chatCalls(events, "2026-11-06T10:02:00Z").length, 0);
  const plan = { key: 0, tool: "TodoWrite", done: true, input: { todos: [{ content: "One", status: "completed" }] } };
  assert.deepEqual(chatSteps([plan, ...calls]).steps.map((s) => [s.title, s.state]), [["One", "done"], ["Building an interactive app", "active"]]);
});

test("a command wrapped in a login shell is judged by the command inside", () => {
  const shell = (command) => chatStatusOf({ tool: "shell", input: { command } }).kind;
  assert.equal(shell("/bin/zsh -lc 'sed -n 1,5p main.go'"), "reading-code");
  assert.equal(shell("/bin/bash -lc \"git status\""), "repo-history");
  assert.equal(shell("/bin/zsh -lc pwd"), "terminal");
});

test("stepDetail names files and searches but never paths or commands", () => {
  const cases = [
    [{ tool: "Read", input: { file_path: "/Users/me/secret/project/app.tsx" } }, "Reading app.tsx"],
    [{ tool: "Edit", input: { file_path: "/a/b.go" } }, "Editing b.go"],
    [{ tool: "Write", input: { file_path: "/a/new.md" } }, "Writing new.md"],
    [{ tool: "Grep", input: { pattern: "foldSteps" } }, "Searching “foldSteps”"],
    [{ tool: "Glob", input: { pattern: "**/*.go" } }, "Finding “**/*.go”"],
    [{ tool: "apply_patch", input: { files: ["update /r/a.go", "add /r/b.go", "delete /r/c.go"] } }, "Editing a.go and 2 more"],
    [{ tool: "mcp__lazyagents__read_page", input: { ref: "Eng/Plan" } }, "Reading Plan"],
    [{ tool: "WebFetch", input: { url: "https://www.example.com/private?token=x" } }, "Reading example.com"],
    [{ tool: "WebSearch", input: { query: "tts pricing" } }, "Searching the web for “tts pricing”"],
    [{ tool: "Bash", input: { command: "API_KEY=x go test ./..." } }, "Running tests"],
    [{ tool: "shell", input: { command: "curl -H 'Authorization: x' host" } }, "Working in the terminal"],
    [{ tool: "Read", input: { file_path: "/w/.lazyagents/attachments/generated/a.png" } }, "Looking at the image"],
    [{ tool: "Read", input: {} }, "Reading the code"],
    [{ tool: "mcp__other__mystery", input: { a: "b" } }, "Working on it"],
  ];
  for (const [call, want] of cases) assert.equal(stepDetail(call), want, call.tool);
});

test("only the active step carries a detail, skipping bookkeeping and repeats", () => {
  const read = (file_path) => ({ tool: "Read", input: { file_path } });
  const todo = { tool: "TodoWrite", input: { todos: [{ content: "One", status: "completed" }, { content: "Two", status: "in_progress" }, { content: "Three", status: "pending" }] } };
  assert.deepEqual(stepsOf([todo, read("/a/one.ts"), read("/a/two.ts"), { tool: "ToolSearch" }]).steps.map((s) => s.detail), [undefined, "Reading two.ts", undefined]);
  assert.deepEqual(stepsOf([{ tool: "Edit" }, read("/a/x.ts")]).steps.map((s) => s.detail), [undefined, "Reading x.ts"]);
  assert.equal(stepsOf([{ tool: "Bash", input: { command: "go test ./..." } }]).steps[0].detail, undefined);
  assert.equal(stepsOf([todo]).steps[1].detail, undefined);
  const events = [{ seq: 1, type: "tool_call", at: "2026-11-06T10:01:00Z", payload: { id: "a", tool: "Read", input: { file_path: "/a/x.ts" } } }, { seq: 2, type: "lifecycle", at: "2026-11-06T10:01:01Z", payload: { state: "reply_block", kind: "app" } }];
  assert.deepEqual(chatSteps(chatCalls(events)).steps.map((s) => s.detail), [undefined, undefined]);
});

test("stepDetail names canvases, workflows, tickets, images, personas, runs and GitHub work", () => {
  const cases = [
    [{ tool: "mcp__lazyagents__canvas_create", input: { title: "Rail mockups", body: "secret" } }, "Drafting “Rail mockups”"],
    [{ tool: "mcp__lazyagents__canvas_patch", input: { canvas_id: "c9", note: "tighten the intro" } }, "Revising: tighten the intro"],
    [{ tool: "mcp__lazyagents__propose_workflow_edit", input: { name: "Map maker" } }, "Sketching “Map maker”"],
    [{ tool: "mcp__lazyagents__propose_workflow_edit", input: { workflow_id: "w9", summary: "Adds a gate" } }, "Sketching: Adds a gate"],
    [{ tool: "mcp__lazyagents__create_ticket", input: { title: "Fix login" } }, "Drafting “Fix login”"],
    [{ tool: "mcp__lazyagents__create_epic", input: { name: "Billing" } }, "Drafting “Billing”"],
    [{ tool: "mcp__lazyagents__generate_image", input: { prompt: "a cat on a bike" } }, "Creating “a cat on a bike”"],
    [{ tool: "mcp__lazyagents__propose_persona", input: { name: "Nora", instructions: "secret" } }, "Sketching “Nora”"],
    [{ tool: "mcp__lazyagents__list_runs", input: { ticket: "PAY-214" } }, "Checking runs of PAY-214"],
    [{ tool: "mcp__lazyagents__get_video_transcript", input: { url: "https://youtu.be/x", from_seconds: 272 } }, "Watching from 4:32"],
    [{ tool: "mcp__github__create_pull_request", input: { owner: "o", repo: "r", title: "Add rail", body: "secret" } }, "Working on “Add rail”"],
    [{ tool: "mcp__github__get_issue", input: { owner: "o", repo: "r", issue_number: 12 } }, "Working on o/r#12"],
    [{ tool: "mcp__lazyagents__save_memory", input: { body: "private" } }, "Making a note"],
    [{ tool: "mcp__lazyagents__canvas_read", input: { canvas_id: "unknown" } }, "Looking at a canvas"],
  ];
  for (const [call, want] of cases) assert.equal(stepDetail(call), want, call.tool);
});

test("tools that take an id use the name an earlier call in the turn gave it", () => {
  const detail = (calls) => stepsOf(calls).steps.at(-1).detail;
  const created = { tool: "mcp__lazyagents__canvas_create", input: { title: "Rail" }, output: JSON.stringify({ canvas_id: "c1", version: 1 }) };
  assert.equal(detail([created, { tool: "mcp__lazyagents__canvas_patch", input: { canvas_id: "c1", note: "n" } }]), "Revising “Rail”");
  const listed = { tool: "list_workflows", input: {}, output: JSON.stringify({ content: [{ type: "text", text: JSON.stringify({ workflows: [{ id: "w1", name: "Maps" }] }) }] }) };
  assert.equal(detail([listed, { tool: "read_workflow", input: { workflow_id: "w1" } }]), "Looking over “Maps”");
  const runs = { tool: "list_runs", input: {}, output: JSON.stringify({ runs: [{ id: "r1", ticket: "PAY-1" }] }) };
  assert.equal(detail([runs, { tool: "get_run", input: { id: "r1" } }]), "Checking PAY-1");
  const clipped = { tool: "canvas_read", input: { canvas_id: "c2" }, output: '{"body":"long and cut' };
  // No name to show, so the detail would only repeat the step's title.
  assert.equal(detail([clipped, { tool: "canvas_comment", input: { canvas_id: "c2" } }]), undefined);
});
