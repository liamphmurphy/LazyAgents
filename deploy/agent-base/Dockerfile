# syntax=docker/dockerfile:1
# The image every managed run's turns execute in (PLAN.md §6.5): git, gh,
# common toolchains and both harness CLIs. Projects can extend it.
FROM node:24-bookworm-slim

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

# The node user (uid 1000) runs turns. A new named volume copies /workspace's
# ownership from the image, so the agent can write to it.
RUN mkdir -p /workspace /.lazyagents/opencode /run/lazyagents-opencode && chown node:node /workspace /.lazyagents/opencode
USER node
ENV HOME=/home/node
WORKDIR /workspace
CMD ["sh"]
