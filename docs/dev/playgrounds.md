# ADR: Playgrounds

Status: Accepted (2026-11-10). The follow-up named in
[test-environments.md](test-environments.md): a branch's live UI and API, as
something that outlives the turn.

## Context

An agent that builds a UI can only describe how to run it; the person then
checks out the branch and starts it themselves, with their own database. The
turn already has everything to run the app (its workspace, a sandboxed Docker,
the repository's test environment), but everything a turn starts is removed
when it ends, and nothing a turn starts is reachable from the browser.

## Decision

1. **One playground per chat, owned by the runner.** The agent declares it
   with `start_playground` (`playground.Spec`: processes, setup, routes,
   services, an OpenAPI path). The API stores it (`playgrounds`); the runner
   that holds the chat's workspace claims it, starts it, and carries out what
   the person or agent asks (`action`: reset, restart, stop). The runner
   stays the only process that touches Docker, and polling the table follows
   the rest of the runner (usage checks, model listings).
2. **Containers like a turn's, on a network of their own.** Services from the
   spec (the repository's test environment by default), a one-shot setup
   container, then one container per process, from the chat's workspace
   volume, with `CapDrop ALL`, `no-new-privileges` and the turn's limits. Not
   the run's home volume: it holds harness logins. The network isn't internal
   (setup installs dependencies). A container per process gives per-process
   logs and exit detection for free.
3. **A gateway in the runner, one port per playground, on Vesper's host.** The
   runner joins each playground's network and reverse-proxies to the processes
   by route (longest prefix). It serves on ports it publishes
   (`RUNNER_PLAYGROUND_PORTS`), so `make dev` (API on the host) and the full
   stack work the same way.
4. **Same host, other port: same site, other origin.** Same site keeps the
   app's own cookies working inside Vesper's iframe (SameSite=Lax cookies
   aren't sent to a cross-site frame, and Safari blocks third-party cookies),
   and makes the browser send the Vesper session, which is how the gateway
   admits the person; no second token. Other origin keeps the app's scripts
   from reading Vesper. Three things close what same-site opens:
   - The API refuses unsafe methods whose `Sec-Fetch-Site` isn't
     `same-origin` or `none` (`httpapi.crossOriginWrite`). SameSite=Strict
     alone would let a same-site page write as the person.
   - The gateway never forwards the `vesper_session` cookie, and drops any
     `Set-Cookie` for it from the app.
   - The gateway answers CORS (with credentials) only for Vesper's own origins
     (`WEB_URL`, `PUBLIC_URL`), for the API and Logs tabs.
5. **The API tab reads an OpenAPI document from the workspace.** The gateway
   serves it as JSON at `/__vesper/openapi` (YAML converted), read on each
   request so it follows the agent's edits. The tab sends requests from
   Vesper's page to the playground's origin with credentials; the gateway
   rewrites `Origin` to the app's own, so they reach the app as its own page's
   would. Descriptions are shown as text: the document is the agent's.

## Alternatives considered

- **Subdomains (`<id>.play.localhost`).** Cleaner isolation, but cross-site to
  `localhost`: the app's own login cookies don't work in the frame, and a
  token handoff is needed instead of the session.
- **A path under Vesper's origin (`/play/<id>/…`).** Breaks every app that
  uses absolute paths, and puts agent-written scripts on Vesper's origin.
- **Proxying through the API.** Under `make dev` the API runs on the host and
  can't join a Docker network; the runner already holds the socket.
- **Promoting a turn's proxy containers.** They're the agent's ad-hoc
  containers, labelled for cleanup, without a process model or routes.
- **Rendering the OpenAPI document with Scalar or Swagger UI.** Neither shows a
  request's logs or (later) an inferred spec's provenance without forking.

## Consequences

- At most as many playgrounds as ports per runner; past that a start fails
  and says so.
- A playground stops after `RUNNER_PLAYGROUND_IDLE_S` without a request, when
  its chat's segment ends and the workspace is removed, and when the runner
  restarts (startup cleanup removes its containers; rows are marked stopped).
- Processes don't see tools the agent installed with Nix during the chat
  (they live in the run's home profile). Declare them in setup.
- "Logs around this request" matches by time, not by request: the app's logs
  carry no id the gateway could set.
- The Preview tab can't drive the frame's history (cross-origin), so there's
  no back or forward, only a path to load and reload.
- The local runner has no playgrounds; it shares the host.

## Send to chat

The Preview tab's **Send to chat** attaches a Markdown report and a
screenshot to the person's next message, as ordinary chat attachments (no new
API).

- The report comes from the gateway's probe, which posts the page's location,
  console errors and failed fetch/XHR calls (with their `X-Request-Id`) to
  Vesper's page. The panel accepts only messages whose origin is the
  playground's, and reads every field as untrusted plain text. Each failed
  request carries the log lines naming its id, or failing that, stderr written
  within 1.5 s of it. The address bar follows the probe's location, which the
  frame's own location can't give across origins.
- The screenshot uses `getDisplayMedia`: the frame is another origin, so the
  page can't draw it, and a script drawing the DOM inside the app misses
  canvas, images and fonts. The browser asks to share the tab on each send.
  Chrome's Region Capture (`CropTarget`) crops the stream to the frame;
  elsewhere the frame is cropped by its viewport rectangle, or the capture is
  sent whole when it doesn't match this tab's viewport. Declining sends the
  report alone, and the report says why there's no screenshot.

## Follow-ups

- An OpenAPI document for apps without one, inferred from the code by the
  persona, labelled as inferred, cached per commit.
- The agent driving the playground (Playwright against the same app).
- Starting a playground from a repository's saved spec without an agent turn.
