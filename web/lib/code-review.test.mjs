import test from "node:test";
import assert from "node:assert/strict";
import { baseName, commentKey, fileTree, pushStateLabel, repoName, reviewMessage } from "./code-review.ts";

test("reviewMessage anchors each comment to file:line, quotes the line and sorts like a review", () => {
  const msg = reviewMessage(
    [
      { path: "web/next.ts", line: 3, side: "new", code: "return next;", body: "Handle a missing value." },
      { path: "server/auth/callback.go", line: 42, side: "new", code: "  next := safeNext(state)", body: "Reject //evil.com.\nAdd a test." },
      { path: "server/auth/callback.go", line: 7, side: "old", code: "http.Redirect(w, r, \"/\")", body: "Why drop this?" },
      { path: "x.go", line: 1, side: "new", code: "", body: "   " },
    ],
    "fix/login-redirect",
  );
  assert.equal(
    msg,
    "Review of `fix/login-redirect` (3 comments). Please address each:\n\n" +
      "- `server/auth/callback.go:7` (removed line)\n  > http.Redirect(w, r, \"/\")\n  Why drop this?\n" +
      "- `server/auth/callback.go:42`\n  > next := safeNext(state)\n  Reject //evil.com.\n  Add a test.\n" +
      "- `web/next.ts:3`\n  > return next;\n  Handle a missing value.",
  );
  assert.equal(reviewMessage([{ path: "a", line: 1, side: "new", code: "", body: "" }]), "", "nothing to send");
  assert.match(reviewMessage([{ path: "a", line: 1, side: "new", code: "", body: "ok" }]), /^Review of your changes \(1 comment\)/);
});

test("commentKey keeps the two sides of a line apart", () => {
  assert.notEqual(commentKey({ path: "a", line: 1, side: "new" }), commentKey({ path: "a", line: 1, side: "old" }));
});

test("fileTree groups by directory, root first, sorted", () => {
  const f = (path) => ({ path, status: "modified", additions: 0, deletions: 0, binary: false, truncated: false });
  const tree = fileTree([f("web/b.ts"), f("README.md"), f("server/z.go"), f("web/a.ts")]);
  assert.deepEqual(tree.map((d) => [d.dir, d.files.map((x) => baseName(x.path))]), [["", ["README.md"]], ["server", ["z.go"]], ["web", ["a.ts", "b.ts"]]]);
});

test("repoName shortens GitHub URLs", () => {
  assert.equal(repoName("https://github.com/liamphmurphy/Vesper.git"), "liamphmurphy/Vesper");
  assert.equal(repoName("git@github.com:a/b.git"), "a/b");
  assert.equal(repoName("/tmp/origin"), "/tmp/origin");
});

test("pushStateLabel says what a card is waiting on", () => {
  assert.equal(pushStateLabel({ state: "staging", auto: false }, "Hackerman"), "Staged when Hackerman's turn ends");
  assert.equal(pushStateLabel({ state: "pushed", auto: true, pr_number: 412 }, "H"), "Pushed automatically · PR #412");
  assert.equal(pushStateLabel({ state: "failed", auto: false }, "H"), "Not pushed");
});
