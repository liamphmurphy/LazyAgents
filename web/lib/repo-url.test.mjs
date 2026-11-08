import test from "node:test";
import assert from "node:assert/strict";
import { checkRepoURL } from "./repo-url.ts";

test("empty is allowed (runs start in an empty workspace)", () => {
  assert.deepEqual(checkRepoURL("  "), { state: "empty" });
});

test("SSH forms are kept as typed", () => {
  for (const u of ["git@github.com:liamphmurphy/Vesper.git", "git@github.com:a/b", "ssh://git@github.com/a/b.git"]) {
    assert.deepEqual(checkRepoURL(u), { state: "ok", kind: "ssh", saved: u });
  }
  assert.deepEqual(checkRepoURL(" git@github.com:a/b.git/ "), { state: "ok", kind: "ssh", saved: "git@github.com:a/b.git" });
});

test("HTTPS GitHub links are saved in SSH form, matching the server", () => {
  const cases = {
    "https://github.com/northwind/payments": "git@github.com:northwind/payments.git",
    "https://github.com/northwind/payments.git": "git@github.com:northwind/payments.git",
    " https://www.github.com/a/b.js/ ": "git@github.com:a/b.js.git",
  };
  for (const [input, saved] of Object.entries(cases)) {
    assert.deepEqual(checkRepoURL(input), { state: "ok", kind: "https", saved });
  }
});

test("non-GitHub and malformed URLs are rejected with a reason", () => {
  assert.match(checkRepoURL("git@gitlab.com:northwind/payments.git").message, /Only GitHub/);
  assert.match(checkRepoURL("https://gitlab.com/a/b").message, /Only GitHub/);
  assert.match(checkRepoURL("http://github.com/a/b.git").message, /https:\/\//);
  for (const u of ["https://github.com/a", "https://github.com/a/b/tree/main", "https://github.com/a/..", "git@github.com:a/..", "git@github.com:a/b?x=1", "-oProxyCommand=x", "https://user@github.com/a/b"]) {
    assert.equal(checkRepoURL(u).state, "invalid", u);
  }
});
