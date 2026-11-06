// Tiny local stand-in for the provider model-discovery endpoints. The API is
// pointed at it via LAZYAGENTS_TEST_DISCOVERY_URL (test-only seam, see
// server/internal/chatmodels/discovery.go), so OpenCode's catalog is
// deterministic and no request ever reaches a real provider.
import { createServer } from "node:http";

const port = Number(process.argv[2]);
const models = {
  "/anthropic/v1/models": [
    { id: "claude-sonnet-5-5", display_name: "Claude Sonnet 5.5 (stub)" },
  ],
  "/openai/v1/models": [{ id: "gpt-5", display_name: "GPT-5 (stub)" }],
};

createServer((req, res) => {
  const path = new URL(req.url ?? "/", "http://stub").pathname;
  if (path === "/healthz") {
    res.writeHead(200).end("ok");
    return;
  }
  const data = models[path];
  if (!data) {
    res.writeHead(404).end();
    return;
  }
  res.writeHead(200, { "content-type": "application/json" });
  res.end(JSON.stringify({ data, has_more: false }));
}).listen(port, "127.0.0.1");
