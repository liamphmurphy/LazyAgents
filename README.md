<div align="center">
  <img src="web/public/logo.svg" width="88" height="88" alt="LazyAgents logo" />
  <h1>LazyAgents</h1>
  <p><strong>One shared harness for your coding agents, whichever model runs them.</strong></p>
  <p>Plan the work, mock up the UI, dispatch tickets, and review what needs you, all from the browser.</p>
  <p>
    <a href="#a-shared-harness-for-every-model">Why LazyAgents</a> ·
    <a href="#what-you-can-do">Features</a> ·
    <a href="#get-started">Get started</a> ·
    <a href="docs/guide.md">Documentation</a>
  </p>
  <p><strong>Self-hosted</strong> &nbsp; / &nbsp; Claude Code · Codex · OpenCode &nbsp; / &nbsp; Automatic fallbacks &nbsp; / &nbsp; Human approvals</p>
</div>

![LazyAgents dashboard showing setup status, items that need you, live runs, blocked tickets, active chats, and epics at risk](docs/images/homepage.png)

## A shared harness for every model

LazyAgents is a control plane for coding agents. You define an agent once (role,
instructions, skills, scopes, model) and LazyAgents runs it headless on the
frontier labs' own harnesses: **Claude Code**, **Codex**, and optionally
**OpenCode**. Every agent reports back through the same MCP server, so tickets,
plans, approvals, knowledge, and costs look the same no matter which model did
the work.

- **No CLI required.** You work in the web app. Managed runs execute in
  containers from the `lazyagents/agent-base` image, which ships the pinned
  harness CLIs and authenticates with the provider keys you store in
  **Settings → Connections & secrets**. Nobody needs to install a CLI, log in,
  or keep a terminal open. If you prefer your own logins, a local runner can use
  the CLIs on your machine instead.
- **Run it on your laptop or a server.** The whole stack (API, web, runner,
  Postgres, object storage, Caddy) is a single `docker compose up`. Run it on
  your machine or on a shared box, and point `PUBLIC_URL` at it so agents can
  reach its MCP endpoint. See [Current scope](#current-scope) before exposing it.
- **Automatic hand-off between models.** Give an agent an ordered list of
  **usage fallbacks**, for example Claude Code first, then Codex. When a
  provider hits a quota or rate limit, the run moves to the next target on the
  same checkout and plan, with a bounded transcript of recent messages, tool
  calls, and approval decisions. Chats can switch harness or model mid-conversation
  in the same way.
- **Agents learn as they go.** Agents look up team conventions with
  `find_pattern` before they implement, and propose new lessons with
  `propose_pattern` when they find something reusable. A curator agent or a
  person publishes, merges, or rejects each proposal into the shared
  `Patterns/` wiki, so the next run starts from what earlier runs learned,
  whichever model it uses. Project-specific decisions stay in per-project notes
  and ADRs.
- **Plan first, mock up before you build.** A planner agent turns a goal, epic,
  or PRD into draft tickets with dependencies, which you edit and accept. Flag
  an epic or ticket as UI work and the agent must draft an SVG mockup and wait
  for your approval before it can open a PR or finish the ticket.
- **Humans stay in charge.** Every dispatch needs your approval. Scopes decide
  what each agent may do, and gated actions pause the run until you answer.

## What you can do

| | |
| :--- | :--- |
| **Turn an idea into a plan** | Start a plan from a goal, an epic, or a markdown PRD/spec, with wiki pages, artifacts, and tickets attached as context. Review, edit, drop, or reassign the proposals and set dependencies before tickets are created. |
| **Review mockups** | Approve or request changes on agent-drafted UI mockups. Zoom, compare versions, and pin comments to the exact spot. The waiting run resumes with your decision. |
| **Keep projects moving** | Organize work into projects, epics, and tickets, and track it on the board, in saved list views, and on an epic's dependency map. Tickets in an epic ship as one stacked pull request. |
| **Build your agent team** | Give agents roles, instructions, reusable skills, scopes, memory, a model, and usage fallbacks. Main agents can delegate to sub agents, in parallel worktrees when the work splits cleanly. |
| **Stay in the conversation** | Chat with any available harness and model, attach context with `[[`, and save replies to the wiki or as artifacts. |
| **Keep knowledge together** | A wiki with a namespace tree, backlinks, versions, and tagged patterns, linked to the tickets and projects that use it. |
| **Review from one inbox** | Plans, dispatch requests, approvals, mockup reviews, blockers, and work awaiting review in one keyboard-driven list. |
| **See what happened** | Follow live runs, timelines, sub-agent trees, and the runner environment. See spend by agent, project, model, and purpose where pricing is known. |

## From idea to review

1. **Describe the goal.** Start a plan, or write a ticket with the context an agent needs.
2. **Shape the plan.** Accept the planner's tickets, arrange dependencies, and assign agents.
3. **Approve the mockup.** For UI work, review the agent's mockup before it builds.
4. **Review and dispatch.** Approve each ticket when it is ready to run.
5. **Follow along.** Answer approval requests, review the PR, and let lessons flow into the wiki.

## Get started

You need Docker (Docker Desktop or OrbStack on a laptop, or Docker Engine on a server).

```sh
git clone https://github.com/liamphmurphy/LazyAgents.git
cd LazyAgents
cp .env.example .env
docker compose up --build
```

Open **[localhost:8080](http://localhost:8080)**. The first boot creates a demo
workspace so you can explore before connecting an agent, and a setup checklist
walks you through what is still missing. To start with an empty workspace, set
`SEED=false` and `LOCAL_USER_NAME` in `.env` before first boot.

Ready to run agents? Set `SECRETS_KEY` in `.env`, then follow the
**[setup guide](docs/guide.md#running-agents)** to add provider keys
(`harness:anthropic` for Claude Code, `harness:openai` for Codex) and connect a
repository. Agent runs use your configured providers and can incur model costs.

### Current scope

LazyAgents does not have a login yet: everyone who can reach it acts as the
workspace's first user. The Docker runner also mounts the Docker socket, which
is root-equivalent on the host. On a server, keep it on a private network or
behind your own authenticating proxy, and only run projects you trust. See the
[guide](docs/guide.md) before running it.

## Explore further

- **[Running and developing](docs/guide.md)**: setup, chat, skills, patterns, planning, dependencies, fallbacks, and development commands.
- **[Kanban board](docs/board.md)**: statuses, estimates, and the board's agent tools.
- **[OpenCode harness](docs/opencode.md)**: install, auth, isolation, and limits.
- **[Example skills](examples/skills/)**: starter playbooks for your agents.
- **[Project plan](PLAN.md)**: architecture and implementation details.
- **[Design](lazyagents-design/)**: the app's design references.

Have an idea or found something that could work better? [Open an issue](https://github.com/liamphmurphy/LazyAgents/issues).
