# ADR: Test environments without a Vesper file

Status: Accepted (2026-11-10). Follow-up: live previews of a branch's UI, as a
service whose lifetime is the branch rather than the turn.

## Context

Agents reported "tests pass" without running the tests that mattered:

- `testdb.New` skipped when `TEST_DATABASE_URL` was unset, and Docker turns
  only had it when a person declared a Postgres service on the project.
- A turn runs as uid 1000 with no capabilities and no Docker socket (the
  socket is root on the host), so it couldn't start a database, install a
  tool with apt, or use Testcontainers.
- Declared services and `.lazyagents-deps.json` both require the repository's
  owner to write Vesper-specific configuration, which most won't.

The goal is that an agent can get any dependency a repository's tests need,
with nothing added to the repository, while damage stays inside containers.

## Decision

1. **Fail instead of skip.** DB-backed tests fail inside a run
   (`LAZYAGENTS_RUN_ID`) or on CI when there's no database.
2. **A Docker API per turn, through a sandboxing proxy** (`runner/docker_proxy.go`).
   Any image is allowed; the line held is the container boundary. The proxy
   forwards an allowlist of Engine calls, rebuilds `HostConfig` from fields that
   only shape the container, refuses anything that reaches the host
   (privileged, capabilities, devices, host namespaces, host paths, foreign
   volumes and networks, volume driver options), labels what it creates and
   scopes every call to that label. Ports publish on the turn network's gateway
   only. Identity is the socket: one per turn, in a directory turns can
   traverse but not list.
3. **Nix as the tool installer** (`runner/docker_nix.go`), through one
   `nix-daemon` per runner on a shared store volume, so turns keep their
   sandbox and installs are cached across runs.
4. **A recipe per repository, inferred then learned** (`internal/testenv`,
   table `test_environments`). Inference reads CI service containers (then
   well-known backing services in compose files); learning adds the Nix
   packages each turn's profile holds, read from the stopped container. People
   can view, replace and forget it in Settings. Project-declared services win.
5. **Services get Docker's default capabilities**, matching proxy containers,
   so official images (and recipes read from CI, which name no user) work.

## Alternatives considered

- **Sysbox (Docker inside the turn):** the cleanest isolation, but a host
  runtime install that Docker Desktop and OrbStack on macOS don't support.
- **Rootless Podman in the turn:** needs user namespaces and `newuidmap`, so
  the turn would lose `CapDrop ALL` and `no-new-privileges`.
- **apt as root in the turn:** natural for agents, but needs root or sudo
  (blocked by `no-new-privileges`), and installs vanish with each turn.
- **An endpoint-filtering socket proxy** (e.g. docker-socket-proxy): can't stop
  `Binds: ["/:/host"]` in a create body.
- **A required repo file (`.vesper/env.json`, `flake.nix`):** rejected; the
  owner shouldn't have to write one. A recipe can be exported as one later.

## Consequences

- An agent can run anything as root inside a container with default
  capabilities. Escaping it takes a kernel or runtime bug. That trade was
  chosen explicitly over approval prompts and image allowlists.
- Containers started through the proxy can reach the internet (cloud
  emulators fetch runtimes); declared services still can't.
- Proxy containers, like services, last one turn. A chat's next message
  starts them again.
- Compose works: checkout binds are rewritten to the run's own checkout
  (volume subpath, or a symlink-checked host-folder path), and builds use the
  classic builder on the turn's network. Not supported: BuildKit (opaque
  session/gRPC streams that can grant host networking), swarm, plugins,
  removing images the run didn't build, `update` (it would lift limits), Ryuk, and non-upgraded attach
  streams. Pulls and build tags can't replace locally built images, closing a
  path to swapping the agent image every turn runs in.
- Images built by turns stay in the daemon's store (a build cache for the next
  turn) until the run removes them itself; nothing sweeps them yet.
- A recipe never drops a package on its own: one turn not using a tool isn't
  evidence the tests don't need it. Forget resets it.
- The local runner gets none of this; it shares the host.

## Open questions

- Learning services, not just tools: the proxy knows which images a turn ran,
  but not which environment variables the tests then needed.
- The delegated-run test check (`checkChild`) runs without services or Docker.
- Nix upgrades need the store volume removed by hand.
