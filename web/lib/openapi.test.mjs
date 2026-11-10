import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { basePath, curlFor, exampleFor, filterOps, parseOpenAPI, requestPath, validate } from "./openapi.ts";

const doc = {
  openapi: "3.0.3",
  info: { title: "Vesper" },
  servers: [{ url: "/api/v1" }],
  tags: [{ name: "runs" }, { name: "chats" }],
  paths: {
    "/runner/test-environments": {
      get: { operationId: "listTestEnvironments", tags: ["runs"], responses: {} },
      put: {
        operationId: "putTestEnvironment",
        summary: "Replace a repository's test environment",
        tags: ["runs"],
        requestBody: { required: true, content: { "application/json": { schema: { $ref: "#/components/schemas/TestEnvironmentInput" } } } },
        responses: {},
      },
      delete: { operationId: "deleteTestEnvironment", tags: ["runs"], parameters: [{ name: "repo", in: "query", required: true, schema: { type: "string" } }], responses: {} },
    },
    "/chats/{id}/playground/{action}": {
      parameters: [{ $ref: "#/components/parameters/ChatId" }],
      post: { operationId: "chatPlaygroundAction", tags: ["chats"], parameters: [{ name: "action", in: "path", schema: { type: "string", enum: ["start", "reset", "stop"] } }], responses: {} },
    },
    "/health": { get: { responses: {} } },
  },
  components: {
    parameters: { ChatId: { name: "id", in: "path", required: true, schema: { type: "string", format: "uuid" } } },
    schemas: {
      TestEnvironmentInput: { type: "object", required: ["repo", "recipe"], properties: { repo: { type: "string" }, recipe: { $ref: "#/components/schemas/TestRecipe" } } },
      TestRecipe: { type: "object", properties: { services: { type: "array", items: { type: "object", properties: { image: { type: "string", example: "postgres:17" } } } }, nix_packages: { type: "array", items: { type: "string" } } } },
    },
  },
};

test("operations are grouped by tag in the document's order", () => {
  const api = parseOpenAPI(doc);
  assert.equal(api.base, "/api/v1");
  assert.equal(api.count, 5);
  assert.deepEqual(api.groups.map((g) => g.tag), ["runs", "chats", "other"]);
  const action = api.groups[1].ops[0];
  assert.deepEqual(action.params.map((p) => `${p.in}:${p.name}:${p.required}`), ["path:id:true", "path:action:true"]);
  const put = api.groups[0].ops.find((o) => o.method === "PUT");
  assert.equal(put.body.contentType, "application/json");
  assert.equal(put.body.required, true);
  assert.throws(() => parseOpenAPI({ openapi: "3" }), /no paths/);
});

test("filtering matches every word", () => {
  const api = parseOpenAPI(doc);
  assert.deepEqual(filterOps(api, "put test").flatMap((g) => g.ops.map((o) => o.operationId)), ["putTestEnvironment"]);
  assert.deepEqual(filterOps(api, "chats").map((g) => g.tag), ["chats"]);
});

test("an example body follows the schema through refs", () => {
  const body = exampleFor(doc, doc.paths["/runner/test-environments"].put.requestBody.content["application/json"].schema);
  assert.deepEqual(body, { repo: "string", recipe: { services: [{ image: "postgres:17" }], nix_packages: ["string"] } });
});

test("validation warns where the body doesn't match", () => {
  const schema = { $ref: "#/components/schemas/TestEnvironmentInput" };
  assert.deepEqual(validate(doc, schema, { repo: "x", recipe: { nix_packages: null } }), [{ path: "recipe.nix_packages", message: "expected array, got null" }]);
  assert.deepEqual(validate(doc, schema, { recipe: {} }), [{ path: "repo", message: "is required" }]);
  assert.deepEqual(validate(doc, { type: "string", enum: ["a"] }, "b"), [{ path: "body", message: 'must be one of "a"' }]);
  assert.deepEqual(validate(doc, schema, { repo: "x", recipe: {} }), []);
});

test("the request path fills and encodes parameters", () => {
  const api = parseOpenAPI(doc);
  const action = api.groups[1].ops[0];
  assert.equal(requestPath(api.base, action, { "path:id": "a/b", "path:action": "stop" }), "/api/v1/chats/a%2Fb/playground/stop");
  const del = api.groups[0].ops.find((o) => o.method === "DELETE");
  assert.equal(requestPath(api.base, del, { "query:repo": "github.com/acme/app" }), "/api/v1/runner/test-environments?repo=github.com%2Facme%2Fapp");
});

test("the base is the server's path, whatever its host", () => {
  assert.equal(basePath({ servers: [{ url: "https://api.example.com/v2/" }] }), "/v2");
  assert.equal(basePath({ servers: [{ url: "{scheme}://x/api" }] }), "/api");
  assert.equal(basePath({}), "");
});

test("curl quotes what it's given", () => {
  assert.equal(curlFor("PUT", "http://localhost:8091/a", { "Content-Type": "application/json" }, `{"q":"it's"}`), `curl -X PUT 'http://localhost:8091/a' -H 'Content-Type: application/json' --data '{"q":"it'\\''s"}'`);
});

// Vesper's own document, as the gateway would serve it.
test("Vesper's own OpenAPI document parses", async (t) => {
  let yaml;
  try {
    yaml = (await import("js-yaml")).default;
  } catch {
    return t.skip("js-yaml isn't installed");
  }
  const real = yaml.load(readFileSync(new URL("../../api/openapi.yaml", import.meta.url), "utf8"));
  const api = parseOpenAPI(real);
  assert.equal(api.base, "/api/v1");
  assert.ok(api.count > 200, `${api.count} operations`);
  const put = api.groups.flatMap((g) => g.ops).find((o) => o.operationId === "putTestEnvironment");
  assert.equal(put.key, "PUT /runner/test-environments");
  const body = exampleFor(real, put.body.schema);
  assert.equal(typeof body.repo, "string");
  assert.deepEqual(validate(real, put.body.schema, body), []);
});
