// Code and Markdown snippets for the chat composer: a small rule-based language guess,
// the fence a snippet is sent in, and when a paste should be offered as a snippet or a
// file. No React here so node --test can cover it.

/** A snippet waiting in the composer; `lang` is a SNIPPET_LANGS tag. */
export type Snippet = { key: number; text: string; lang: string };

/**
 * The languages the snippet picker offers, as fence tags. Each tag is one CodeFence
 * highlights (lib/code-diff.ts BY_FENCE), except diff, log and text, which render plain.
 */
export const SNIPPET_LANGS: { tag: string; label: string }[] = [
  { tag: "text", label: "Plain text" },
  { tag: "markdown", label: "Markdown" },
  { tag: "log", label: "Log" },
  { tag: "diff", label: "Diff" },
  { tag: "bash", label: "Shell" },
  { tag: "c", label: "C" },
  { tag: "cpp", label: "C++" },
  { tag: "csharp", label: "C#" },
  { tag: "css", label: "CSS" },
  { tag: "dockerfile", label: "Dockerfile" },
  { tag: "go", label: "Go" },
  { tag: "html", label: "HTML" },
  { tag: "java", label: "Java" },
  { tag: "js", label: "JavaScript" },
  { tag: "json", label: "JSON" },
  { tag: "jsx", label: "JSX" },
  { tag: "kotlin", label: "Kotlin" },
  { tag: "makefile", label: "Makefile" },
  { tag: "php", label: "PHP" },
  { tag: "python", label: "Python" },
  { tag: "ruby", label: "Ruby" },
  { tag: "rust", label: "Rust" },
  { tag: "scss", label: "SCSS" },
  { tag: "sql", label: "SQL" },
  { tag: "swift", label: "Swift" },
  { tag: "toml", label: "TOML" },
  { tag: "ts", label: "TypeScript" },
  { tag: "tsx", label: "TSX" },
  { tag: "yaml", label: "YAML" },
];

export function snippetLangLabel(tag: string): string {
  return SNIPPET_LANGS.find((l) => l.tag === tag)?.label ?? tag;
}

/** A paste this long is offered as a file instead: it would flood the message and the transcript. */
export const BIG_PASTE_LINES = 400;
export const BIG_PASTE_CHARS = 40_000;
/** Fewer lines than this is a phrase or a one-liner, not worth offering as a snippet. */
export const SNIPPET_OFFER_MIN_LINES = 3;

export const lineCount = (text: string) => (text === "" ? 0 : text.replace(/\n+$/, "").split("\n").length);

export const isBigPaste = (text: string) => lineCount(text) > BIG_PASTE_LINES || text.length > BIG_PASTE_CHARS;

// Each language's tells and what each is worth when present (counted once, not per hit).
// Weights favour constructs no other listed language shares; shared ones (=>, ;) stay small.
type Rule = [RegExp, number];
const RULES: Record<string, Rule[]> = {
  go: [
    [/^package \w+\s*$/m, 6],
    [/^import \($/m, 3],
    [/\bfunc\s+(\([^)]*\)\s*)?\w+\([^)]*\)\s*(\(|\*?[\w.[\]]+\s*)?\{/, 3],
    [/:=/, 2],
    [/\berr != nil\b/, 4],
    [/\bfmt\.\w+\(/, 3],
    [/\b(go func|chan |defer )/, 2],
  ],
  python: [
    [/^\s*def \w+\(.*\)(\s*->\s*[^:]+)?:\s*$/m, 5],
    [/^\s*class \w+(\(.*\))?:\s*$/m, 4],
    [/^\s*(elif|except|finally|with)\b.*:\s*$/m, 3],
    [/^\s*from [\w.]+ import \w/m, 4],
    [/\bself\.\w+/, 2],
    [/\b(None|True|False)\b/, 1],
    [/\bprint\(/, 1],
    [/^\s*(if|for|while)\s.*:\s*$/m, 2],
    [/\bin range\(/, 3],
  ],
  js: [
    [/\b(const|let|var)\s+[\w{[][^=]*=/, 2],
    [/=>/, 2],
    [/\bfunction\s*\w*\s*\(/, 2],
    [/\bconsole\.\w+\(/, 3],
    [/\brequire\(['"]/, 3],
    [/^\s*import\s.*\sfrom\s+['"]/m, 3],
    [/^\s*export\s+(default|const|function|class|async)\b/m, 3],
    [/===|!==/, 2],
    [/\b(document|window)\.\w+/, 2],
    [/\basync\s+(function|\(|\w+\s*=>)|\bawait\s/, 1],
  ],
  // Added on top of js's score when present; decides ts over js.
  ts: [
    [/[\w)]\s*:\s*(string|number|boolean|void|unknown|any|never)(\[\])?\b/, 3],
    [/^\s*(export\s+)?(interface|type)\s+\w+(<[^>]*>)?\s*(=|\{)/m, 4],
    [/\bas\s+(const|string|number|unknown)\b/, 3],
    [/\b(readonly|private|public|protected)\s+\w+\s*[:;=]/, 2],
    [/^\s*import\s+type\s/m, 4],
  ],
  rust: [
    [/\bfn\s+\w+\s*(<[^>]*>)?\(/, 4],
    [/\blet\s+mut\b/, 4],
    [/^\s*(pub\s+)?(impl|trait|enum|struct|mod)\b/m, 2],
    [/^\s*use\s+[\w:]+(::\{[^}]*\})?;/m, 4],
    [/\b\w+!\(/, 2],
    [/&(mut\s+)?self\b/, 3],
    [/->\s*(Result|Option|Self|Vec|String|&?str|[iu](8|16|32|64|size)|bool)\b/, 3],
  ],
  java: [
    [/\bpublic\s+(static\s+)?(final\s+)?(class|void|interface)\b/, 4],
    [/\bSystem\.out\.print/, 5],
    [/^\s*import\s+java\./m, 6],
    [/^\s*package\s+[\w.]+;\s*$/m, 5],
    [/@Override\b/, 3],
    [/\b(private|protected)\s+(static\s+)?(final\s+)?[\w<>[\]]+\s+\w+\s*[;=(]/, 2],
  ],
  kotlin: [
    [/\bfun\s+(<[^>]*>\s*)?[\w.]+\(/, 5],
    [/\bval\s+\w+(\s*:\s*\w+)?\s*=/, 2],
    [/\bdata class\b/, 4],
    [/^\s*package\s+[\w.]+\s*$/m, 2],
  ],
  swift: [
    [/^\s*import\s+(UIKit|SwiftUI|Foundation|Combine)\s*$/m, 6],
    [/\bfunc\s+\w+\([^)]*:\s*[A-Z][^)]*\)/, 3],
    [/\bguard\s+let\b|\bif\s+let\b/, 4],
    [/\bstruct\s+\w+\s*:\s*\w+/, 3],
    [/\)\s*->\s*[A-Z]\w*\??\s*\{/, 2],
  ],
  c: [
    [/^#include\s*<\w+\.h>/m, 5],
    [/\bprintf\(/, 2],
    [/\bint\s+main\s*\(/, 3],
    [/\b(malloc|free|sizeof)\(/, 2],
  ],
  cpp: [
    [/^#include\s*<\w+>/m, 5],
    [/\bstd::/, 5],
    [/\b(cout|cerr)\s*<</, 3],
    [/\btemplate\s*</, 3],
    [/^\s*namespace\s+\w+/m, 2],
  ],
  csharp: [
    [/^\s*using\s+System(\.[\w.]+)?;/m, 6],
    [/\bConsole\.Write/, 5],
    [/\{\s*get;\s*(set;\s*)?\}/, 4],
    [/\bpublic\s+(async\s+)?(Task|void|string|int|bool)(<[^>]*>)?\s+\w+\(/, 2],
    [/^\s*namespace\s+[\w.]+/m, 2],
  ],
  php: [
    [/^<\?php/, 10],
    [/\$\w+\s*=[^=]/, 2],
    [/\bfunction\s+\w+\s*\(\s*\$/, 4],
    [/\$this->/, 4],
  ],
  ruby: [
    [/^\s*def\s+(self\.)?\w+[?!]?(\(.*\))?\s*$/m, 3],
    [/^\s*end\s*$/m, 3],
    [/\.each\s+do\b|\bdo\s+\|\w+(,\s*\w+)*\|/, 4],
    [/^\s*require\s+['"]/m, 3],
    [/^\s*(module|class)\s+[A-Z]\w*(\s*<\s*[\w:]+)?\s*$/m, 2],
    [/\bputs\s/, 2],
  ],
  bash: [
    [/^#!.*\b(ba|z)?sh\b/, 10],
    [/^\s*\$ \S/m, 3],
    [/^\s*(sudo|npm|npx|yarn|pnpm|git|docker|kubectl|curl|wget|cd|ls|mkdir|rm|cp|mv|chmod|export|echo|brew|apt|apt-get|pip|make|go|cargo)\s/m, 3],
    [/^\s*(fi|done|esac)\s*$/m, 4],
    [/^\s*if \[\[? /m, 4],
    [/\|\s*(grep|awk|sed|xargs|head|tail|jq|sort|wc)\b/, 3],
    [/\$\{\w+[^}]*\}|\$\(\w/, 2],
  ],
  sql: [
    [/^\s*(SELECT|INSERT\s+INTO|UPDATE|DELETE\s+FROM|CREATE\s+(TABLE|INDEX|VIEW|UNIQUE)|ALTER\s+TABLE|DROP\s+TABLE|WITH\s+\w+\s+AS)\b/im, 5],
    [/\bFROM\s+[\w.]+/i, 1],
    [/\bWHERE\b/i, 1],
    [/\b(JOIN|GROUP BY|ORDER BY|VALUES)\b/i, 2],
  ],
  html: [
    [/^\s*<!doctype html>/i, 10],
    [/<\/?(html|head|body|div|span|section|main|nav|ul|li|table|script|style|template|button|input|form)\b[^>]*>/i, 3],
    [/<\/\w+>/, 1],
  ],
  css: [
    [/^\s*([.#]?[\w-]+|\*)([\s,>+~]+[.#:]?[\w-]+|:[\w-]+(\([^)]*\))?)*\s*\{\s*$/m, 2],
    [/^\s*[a-z-]+\s*:\s*[^;{}]+;\s*$/m, 3],
    [/@(media|import|keyframes|font-face|supports)\b/, 3],
  ],
  // Added on top of css's score when present; decides scss over css.
  scss: [
    [/^\s*\$[\w-]+\s*:/m, 4],
    [/&[:.\w-]/, 3],
    [/@(mixin|include|extend|use)\b/, 4],
  ],
  markdown: [
    [/^#{1,6}\s+\S/m, 2],
    [/^\s*[-*+]\s+\S/m, 1],
    [/^\s*\d+\.\s+\S/m, 1],
    [/\[[^\]\n]+\]\([^)\s]+\)/, 3],
    [/^```/m, 3],
    [/\*\*[^*\n]+\*\*|__[^_\n]+__/, 2],
    [/^>\s/m, 1],
    [/^\|.*\|\s*$/m, 2],
  ],
};

// Below this a guess is a coin toss: say plain text and let the person pick.
const MIN_SCORE = 4;

const nonEmpty = (text: string) => text.split("\n").filter((l) => l.trim() !== "");

const share = (lines: string[], re: RegExp) => lines.filter((l) => re.test(l)).length / Math.max(1, lines.length);

function isJSON(text: string): boolean {
  if (!/^[[{]/.test(text)) return false;
  try {
    JSON.parse(text);
    return true;
  } catch {
    return false;
  }
}

// Whole-snippet shapes that a single tell would undersell; checked before scoring.
function shape(text: string, lines: string[]): string | null {
  if (isJSON(text)) return "json";
  const first = lines[0] ?? "";
  if (/^#!.*\bpython/.test(first)) return "python";
  if (/^#!.*\bnode\b/.test(first)) return "js";
  if (/^#!.*\bruby\b/.test(first)) return "ruby";
  if (/^diff --git /m.test(text) || (/^--- \S/m.test(text) && /^\+\+\+ \S/m.test(text)) || /^@@ -\d+(,\d+)? \+\d+(,\d+)? @@/m.test(text)) return "diff";
  const timestamp = /^\[?\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}|^\[?\d{2}:\d{2}:\d{2}|^(TRACE|DEBUG|INFO|WARN|WARNING|ERROR|FATAL)\b/;
  if (lines.length >= 2 && share(lines, timestamp) >= 0.5) return "log";
  const instruction = /^(FROM|RUN|COPY|ADD|CMD|ENTRYPOINT|WORKDIR|ENV|ARG|EXPOSE|USER|LABEL|VOLUME)\s/;
  const code = lines.filter((l) => !l.trimStart().startsWith("#"));
  if (/^FROM\s+\S+/i.test(code[0] ?? "") && share(code, instruction) >= 0.5) return "dockerfile";
  if (/^\.PHONY:/m.test(text) || /^[\w.%/-]+:[^=\n]*\n\t\S/m.test(text)) return "makefile";
  // Every line `key: value`, `- item` or a comment, nothing a brace language would have, and
  // at least one key holding a nested block: flat `Word: words` lines are usually prose.
  const yamlLine = /^\s*(-\s+|-$|#|[\w."'/-]+:(\s|$)|---$)/;
  const nested = /^\s*[\w."'/-]+:\s*\n(\s+\S|-\s)/m;
  if (lines.length >= 2 && share(lines, yamlLine) >= 0.9 && !/[;{}]\s*$/m.test(text) && nested.test(text)) return "yaml";
  const tomlLine = /^\s*(\[{1,2}[\w.-]+\]{1,2}|[\w.-]+\s*=\s*\S|#)/;
  if (lines.length >= 2 && share(lines, tomlLine) >= 0.9 && /^\s*\[[\w.-]+\]\s*$/m.test(text)) return "toml";
  return null;
}

function score(text: string, lang: string): number {
  return (RULES[lang] ?? []).reduce((sum, [re, weight]) => sum + (re.test(text) ? weight : 0), 0);
}

/**
 * The likeliest SNIPPET_LANGS tag for a paste: obvious shapes first (JSON that parses,
 * a diff, a log, a Dockerfile, YAML), then each language's tells. "text" when nothing
 * scores high enough, since a wrong guess is worse than none: the picker corrects it.
 */
export function detectLanguage(raw: string): string {
  const text = raw.replace(/^\s*\n/, "").trimEnd();
  if (!text.trim()) return "text";
  const lines = nonEmpty(text);
  const shaped = shape(text.trim(), lines);
  if (shaped) return shaped;

  const scores: Record<string, number> = {};
  for (const lang of Object.keys(RULES)) if (lang !== "ts" && lang !== "scss") scores[lang] = score(text, lang);
  // TypeScript and SCSS are supersets: their own tells tip a js or css guess over.
  const ts = score(text, "ts");
  if (ts >= 3) {
    scores.ts = scores.js + ts;
    delete scores.js;
  }
  const scss = score(text, "scss");
  if (scss >= 3) {
    scores.scss = scores.css + scss;
    delete scores.css;
  }
  let best = "text";
  let top = MIN_SCORE - 1;
  for (const [lang, s] of Object.entries(scores)) {
    if (s > top) {
      best = lang;
      top = s;
    }
  }
  // JSX: a script with a capitalised component tag or className.
  if ((best === "js" || best === "ts") && /<[A-Z]\w*[\s/>]|\bclassName=/.test(text)) return best === "ts" ? "tsx" : "jsx";
  return best;
}

/**
 * Whether a paste into the message box is worth offering as a code block: a few lines
 * that read as a language. Markdown and plain text are left alone, since messages
 * already render Markdown.
 */
export function offerAsSnippet(text: string): string | null {
  if (lineCount(text.trim()) < SNIPPET_OFFER_MIN_LINES) return null;
  const lang = detectLanguage(text);
  return lang === "text" || lang === "markdown" ? null : lang;
}

/**
 * A snippet as a Markdown fenced block. The fence is one backtick longer than any run of
 * backticks inside, so a snippet that contains ``` (a README, a chat transcript) can't end
 * its own block early.
 */
export function fenceSnippet(text: string, lang: string): string {
  const body = text.replace(/^\n+/, "").trimEnd();
  const longest = Math.max(0, ...(body.match(/`+/g) ?? []).map((run) => run.length));
  const fence = "`".repeat(Math.max(3, longest + 1));
  return `${fence}${lang}\n${body}\n${fence}`;
}

/** The message to send: the typed text, then each snippet's block in the order they were added. */
export function messageWithSnippets(body: string, snippets: Pick<Snippet, "text" | "lang">[]): string {
  const blocks = snippets.filter((s) => s.text.trim()).map((s) => fenceSnippet(s.text, s.lang));
  if (blocks.length === 0) return body;
  return [body.trim() ? body.trimEnd() : "", ...blocks].filter(Boolean).join("\n\n");
}

/** A name for a paste attached as a file, with an extension chat attachments accept. */
export function pastedSnippetName(lang: string, n: number): string {
  const ext = lang === "json" ? "json" : lang === "markdown" ? "md" : "txt";
  return `pasted-${lang === "text" ? "text" : lang}-${n}.${ext}`;
}
