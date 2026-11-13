import test from "node:test";
import assert from "node:assert/strict";
import {
  BIG_PASTE_LINES,
  detectLanguage,
  fenceSnippet,
  isBigPaste,
  lineCount,
  messageWithSnippets,
  offerAsSnippet,
  pastedSnippetName,
  SNIPPET_LANGS,
  snippetLangLabel,
  withoutPaste,
} from "./snippets.ts";

const SAMPLES = {
  go: `func retry(ctx context.Context, n int) error {
	for i := 0; i < n; i++ {
		if err := call(ctx); err == nil {
			return nil
		}
	}
	return errors.New("gave up")
}`,
  goPackage: `package main

import "fmt"

func main() {
	fmt.Println("hi")
}`,
  python: `def load(path: str) -> dict:
    with open(path) as f:
        return json.load(f)

class Store:
    def __init__(self):
        self.items = None`,
  ts: `export interface User {
  id: string;
  name: string;
}

export const byId = (users: User[], id: string) => users.find((u) => u.id === id);`,
  js: `const express = require("express");
const app = express();
app.get("/", (req, res) => {
  console.log("hit");
  res.send("ok");
});`,
  tsx: `export function Chip({ label }: { label: string }) {
  return <Badge className="chip">{label}</Badge>;
}`,
  jsx: `import React from "react";
export default function App() {
  return <Layout className="app"><Header /></Layout>;
}`,
  rust: `fn main() {
    let mut total = 0;
    for x in vec![1, 2, 3] {
        total += x;
    }
    println!("{}", total);
}`,
  java: `public class Main {
    public static void main(String[] args) {
        System.out.println("hi");
    }
}`,
  kotlin: `data class User(val id: String)

fun greet(user: User) {
    val name = user.id
    println(name)
}`,
  swift: `import SwiftUI

struct ContentView: View {
    var body: some View {
        Text("Hello")
    }
}`,
  c: `#include <stdio.h>

int main(void) {
    printf("hi\\n");
    return 0;
}`,
  cpp: `#include <iostream>

int main() {
    std::cout << "hi" << std::endl;
}`,
  csharp: `using System;

public class Program {
    public static void Main() {
        Console.WriteLine("hi");
    }
}`,
  php: `<?php
function greet($name) {
    echo "Hello $name";
}`,
  ruby: `require "json"

class Store
  def items
    @items.each do |item|
      puts item
    end
  end
end`,
  bash: `#!/usr/bin/env bash
set -euo pipefail
for f in *.log; do
  grep -c ERROR "$f"
done`,
  bashCommands: `cd web
npm ci
npm run build | tail -n 20`,
  sql: `SELECT u.id, count(*)
FROM users u
JOIN orders o ON o.user_id = u.id
WHERE o.created_at > now() - interval '7 days'
GROUP BY u.id;`,
  html: `<!doctype html>
<html>
  <body><div class="app"></div></body>
</html>`,
  css: `.chip {
  display: inline-flex;
  gap: 4px;
}

@media (max-width: 600px) {
  .chip { display: none; }
}`,
  scss: `$gap: 4px;

.chip {
  gap: $gap;
  &:hover { color: red; }
}`,
  json: `{"name": "vesper", "private": true, "scripts": {"dev": "next dev"}}`,
  yaml: `services:
  postgres:
    image: postgres:17-alpine
    ports:
      - "5432:5432"`,
  toml: `[package]
name = "vesper"
version = "0.1.0"

[dependencies]
serde = "1"`,
  dockerfile: `FROM golang:1.25 AS build
WORKDIR /src
COPY . .
RUN go build -o /out/app ./cmd/app`,
  makefile: `.PHONY: test
test:
	go test ./...`,
  diff: `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1,3 +1,3 @@
-old
+new`,
  log: `2026-11-13T09:14:02Z INFO runner: claimed slot 3
2026-11-13T09:14:02Z WARN harness: retry 1/5 after 502
2026-11-13T09:14:05Z ERROR harness: gave up`,
  markdown: `## Retry policy

- Back off **exponentially**
- See [the ADR](docs/adr.md)

\`\`\`yaml
retries: 5
\`\`\``,
};

const EXPECTED = {
  go: "go", goPackage: "go", python: "python", ts: "ts", js: "js", tsx: "tsx", jsx: "jsx", rust: "rust", java: "java",
  kotlin: "kotlin", swift: "swift", c: "c", cpp: "cpp", csharp: "csharp", php: "php", ruby: "ruby", bash: "bash",
  bashCommands: "bash", sql: "sql", html: "html", css: "css", scss: "scss", json: "json", yaml: "yaml", toml: "toml",
  dockerfile: "dockerfile", makefile: "makefile", diff: "diff", log: "log", markdown: "markdown",
};

for (const [name, text] of Object.entries(SAMPLES)) {
  test(`detects ${name}`, () => {
    assert.equal(detectLanguage(text), EXPECTED[name]);
  });
}

test("every guess is a language the picker offers", () => {
  const tags = new Set(SNIPPET_LANGS.map((l) => l.tag));
  for (const lang of Object.values(EXPECTED)) assert.ok(tags.has(lang), lang);
  assert.equal(snippetLangLabel("go"), "Go");
  assert.equal(snippetLangLabel("unknown"), "unknown");
});

test("prose, a phrase and nothing stay plain text rather than a wrong guess", () => {
  assert.equal(detectLanguage(""), "text");
  assert.equal(detectLanguage("   \n  "), "text");
  assert.equal(detectLanguage("hello there"), "text");
  assert.equal(
    detectLanguage("I tried the retry loop again this morning.\nIt still fails after the third attempt,\nand nothing shows in the logs."),
    "text",
  );
  // Looks like JSON but doesn't parse, and has no other tells.
  assert.equal(detectLanguage("{ not json at all }"), "text");
  // Flat `Word: words` lines and bullets are notes, not YAML; YAML needs a nested block.
  assert.equal(detectLanguage("Summary: it broke\nCause: no backoff\nFix: add jitter"), "text");
  assert.equal(detectLanguage("- fix the retry\n- add a test\n- Note: ship it"), "text");
  assert.equal(detectLanguage("apiVersion: v1\nkind: Pod\nmetadata:\n  name: web"), "yaml");
  assert.equal(detectLanguage("for i in range(10):\n    if i % 2 == 0:\n        print(i)"), "python");
});

test("a paste is offered as a code block only for 3+ lines that read as code", () => {
  assert.equal(offerAsSnippet(SAMPLES.go), "go");
  assert.equal(offerAsSnippet(SAMPLES.log), "log");
  assert.equal(offerAsSnippet("x := 1"), null, "one line");
  assert.equal(offerAsSnippet("a\nb"), null, "two lines");
  assert.equal(offerAsSnippet(SAMPLES.markdown), null, "Markdown already renders in messages");
  assert.equal(offerAsSnippet("Thanks for the review.\nI'll fix the naming.\nAnd add a test."), null, "prose");
});

test("a fence is plain ``` with the language tag", () => {
  assert.equal(fenceSnippet("x := 1\n", "go"), "```go\nx := 1\n```");
  assert.equal(fenceSnippet("\n\nx\n\n\n", "text"), "```text\nx\n```", "blank lines around the snippet are dropped");
});

test("a snippet containing ``` gets a longer fence, so it can't close its own block", () => {
  assert.equal(fenceSnippet("```yaml\na: 1\n```", "markdown"), "````markdown\n```yaml\na: 1\n```\n````");
  assert.equal(fenceSnippet("`````", "text"), "``````text\n`````\n``````");
  assert.equal(fenceSnippet("inline `code` here", "markdown"), "```markdown\ninline `code` here\n```");
});

test("the sent message is the typed text, then each snippet's block in order", () => {
  const snippets = [{ text: "x := 1", lang: "go" }, { text: "# Title", lang: "markdown" }];
  assert.equal(messageWithSnippets("Why?\n", snippets), "Why?\n\n```go\nx := 1\n```\n\n```markdown\n# Title\n```");
  assert.equal(messageWithSnippets("   ", snippets.slice(0, 1)), "```go\nx := 1\n```", "a snippet can be the whole message");
  assert.equal(messageWithSnippets("Why?", [{ text: "  \n", lang: "go" }]), "Why?", "empty snippets are dropped");
  assert.equal(messageWithSnippets(" keep as typed ", []), " keep as typed ", "no snippets leaves the text untouched");
});

test("long pastes are offered as a file", () => {
  assert.equal(lineCount(""), 0);
  assert.equal(lineCount("a\nb\n\n"), 2);
  assert.equal(isBigPaste("a\n".repeat(BIG_PASTE_LINES)), false);
  assert.equal(isBigPaste("a\n".repeat(BIG_PASTE_LINES + 1)), true);
  assert.equal(isBigPaste("x".repeat(40_001)), true, "one huge line counts too");
});

test("a pasted file gets an extension chat attachments accept", () => {
  assert.equal(pastedSnippetName("log", 1), "pasted-log-1.txt");
  assert.equal(pastedSnippetName("json", 2), "pasted-json-2.json");
  assert.equal(pastedSnippetName("markdown", 3), "pasted-markdown-3.md");
  assert.equal(pastedSnippetName("go", 4), "pasted-go-4.txt");
});

test("taking a paste back out finds it where it landed, or anywhere, or gives up", () => {
  assert.equal(withoutPaste("Why? x := 1 thanks", { text: "x := 1", start: 5 }), "Why?  thanks");
  assert.equal(withoutPaste("Edited first. x := 1", { text: "x := 1", start: 0 }), "Edited first. ", "moved by edits before it");
  assert.equal(withoutPaste("all gone", { text: "x := 1", start: 0 }), null);
});
