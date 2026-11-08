import assert from "node:assert/strict";
import { test } from "node:test";
import { ancestors, canConnect, canRequestChanges, diffGraphs, gateSources, isWorkflowRunFence, newNodeId, parseWorkflowRun, removeNode } from "./workflows.ts";

const node = (id, type, config = {}) => ({ id, type, title: id, config });
const graph = {
  nodes: [node("start", "trigger"), node("work", "agent"), node("ok", "gate", { retry_from: "work" }), node("page", "wiki")],
  edges: [{ from: "start", to: "work" }, { from: "work", to: "ok" }, { from: "ok", to: "page" }],
};

test("fence holds the workflow id then the request", () => {
  const id = "01a1bbfc-361a-7c32-9fca-75ba4c40636b";
  assert.deepEqual(parseWorkflowRun(`${id}\nCompare queues\nand caches`), { workflowId: id, input: "Compare queues\nand caches" });
  assert.equal(parseWorkflowRun("not-an-id\nhello"), null);
  assert.ok(isWorkflowRunFence("workflow-run"));
  assert.ok(!isWorkflowRunFence("research-plan"));
});

test("connections can't form a cycle or leave an output", () => {
  assert.ok(canConnect(graph, "start", "ok"));
  assert.ok(!canConnect(graph, "ok", "work"));
  assert.ok(!canConnect(graph, "page", "ok"));
  assert.ok(!canConnect(graph, "work", "start"));
  assert.ok(!canConnect(graph, "work", "ok"), "already connected");
});

test("ancestors walks every path back", () => {
  assert.deepEqual(ancestors(graph, "page").map((n) => n.id), ["start", "work", "ok"]);
});

test("removing a node clears references to it", () => {
  const g = removeNode(graph, "work");
  assert.equal(g.edges.length, 1);
  assert.equal(g.nodes.find((n) => n.id === "ok").config.retry_from, undefined);
});

test("diff marks added, changed and removed", () => {
  const next = {
    nodes: [node("start", "trigger"), { ...node("work", "agent"), title: "Renamed" }, node("ok", "gate", { retry_from: "work" }), node("check", "agent")],
    edges: [{ from: "start", to: "work" }, { from: "work", to: "ok" }, { from: "ok", to: "check" }],
  };
  const d = diffGraphs(graph, next);
  assert.equal(d.nodes.get("check"), "added");
  assert.equal(d.nodes.get("work"), "changed");
  assert.equal(d.nodes.get("page"), "removed");
  assert.equal(d.nodes.get("start"), undefined);
  assert.deepEqual(d.addedEdges, [{ from: "ok", to: "check" }]);
  assert.deepEqual(d.removedEdges, [{ from: "ok", to: "page" }]);
});

test("new ids don't collide", () => {
  assert.equal(newNodeId({ nodes: [node("gate_1", "gate")], edges: [] }, "gate"), "gate_2");
});

test("a gate shows its parents unless configured", () => {
  assert.deepEqual(gateSources(graph, graph.nodes[2]), ["work"]);
  assert.deepEqual(gateSources(graph, node("ok", "gate", { show: ["start"] })), ["start"]);
});

test("request changes stops at the limit", () => {
  const gate = node("ok", "gate", { retry_from: "work", max_attempts: 2 });
  assert.ok(canRequestChanges(gate, 1));
  assert.ok(!canRequestChanges(gate, 2));
  assert.ok(!canRequestChanges(node("ok", "gate"), 1));
});
