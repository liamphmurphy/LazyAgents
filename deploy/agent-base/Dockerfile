# syntax=docker/dockerfile:1
# The image every managed run's turns execute in (docs/dev/plan.md §6.5): git, gh,
# common toolchains and both harness CLIs. Projects can extend it.
FROM golang:1.26.0-bookworm AS go-tools
# Build the pinned source as the main module: sqlc uses replace directives,
# which prevent `go install module@version`.
RUN git clone --depth 1 --branch v1.31.1 https://github.com/sqlc-dev/sqlc.git /tmp/sqlc \
 && cd /tmp/sqlc && go build -o /go/bin/sqlc ./cmd/sqlc

FROM node:24-bookworm-slim
COPY --from=go-tools /usr/local/go /usr/local/go
COPY --from=go-tools /go/bin/sqlc /usr/local/bin/sqlc
ENV PATH="/usr/local/go/bin:${PATH}"

RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates curl git openssh-client make jq ripgrep python3 build-essential gnupg \
 && curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg -o /usr/share/keyrings/githubcli.gpg \
 && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli.gpg] https://cli.github.com/packages stable main" \
      > /etc/apt/sources.list.d/github-cli.list \
 && apt-get update && apt-get install -y --no-install-recommends gh \
 && rm -rf /var/lib/apt/lists/*

# Versions the adapters were checked against (server/internal/harness).
ARG CLAUDE_CODE_VERSION=2.1.316
ARG CODEX_VERSION=0.166.0
# The only OpenCode release the adapter supports (docs/contracts/opencode2/ADR.md);
# the runner refuses any other `opencode --version`.
# npm dist.integrity: sha512-U8UIdEOhGSE8PX6iSxOnJroXYst8FC3atXXEcNw+wPe4W7ZWw2FKkhmSDM11ZGdR8WIsxvBkKV57gEmRMG3Vug==
ARG OPENCODE_VERSION=2.2.1
RUN npm install -g --no-fund --no-audit \
      @anthropic-ai/claude-code@${CLAUDE_CODE_VERSION} @openai/codex@${CODEX_VERSION} @opencode/cli@${OPENCODE_VERSION} \
 && npm cache clean --force

# Browser system libraries are installed once in the image. Browser revisions
# follow each project's Playwright lockfile and live in the persistent cache.
ARG PLAYWRIGHT_VERSION=1.64.1
RUN npx --yes playwright@${PLAYWRIGHT_VERSION} install-deps chromium

# The node user (uid 1000) runs turns. A new named volume copies /workspace's
# ownership from the image, so the agent can write to it.
RUN mkdir -p /cache /workspace /.lazyagents/opencode /run/lazyagents-opencode && chown node:node /cache /workspace /.lazyagents/opencode
USER node
ENV HOME=/home/node
WORKDIR /workspace
CMD ["sh"]
