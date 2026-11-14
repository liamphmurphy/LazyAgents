import test from "node:test";
import assert from "node:assert/strict";
import {
  mostUsedPersona,
  AVATAR_MAX_BYTES,
  CHAT_BG,
  MIN_CONTRAST,
  PRESET_COLORS,
  addRepo,
  avatarUploadError,
  checkRepoUrl,
  codesInRepos,
  isImageType,
  repoShortName,
  repositoryChoices,
  sameRepo,
  checkAvatarFile,
  colorPicker,
  contrastRatio,
  contrastWarning,
  deleteConsequence,
  fieldError,
  hexToHsl,
  hslToHex,
  isConflict,
  parseHex,
  personaInitial,
  promotionCandidate,
  sortPersonas,
  unusedPreset,
  wheelPoint,
  wheelPosition,
  withDefault, cssHex } from "./persona-ui.ts";

const p = (id, name, extra = {}) => ({ id, name, color: "#8b8fff", is_default: false, created_at: "2026-11-01T00:00:00Z", ...extra });

test("hex input validates #rrggbb and normalises to lower case", () => {
  assert.equal(parseHex("#5B3FA8"), "#5b3fa8");
  assert.equal(parseHex(" 5b3fa8 "), "#5b3fa8");
  for (const bad of ["", "#5b3fa", "#5b3fa8f", "#ggg000", "rgb(1,2,3)", "#fff"]) assert.equal(parseHex(bad), null, bad);
});

test("hsl and hex round-trip, and the wheel maps angle to hue and radius to saturation", () => {
  for (const c of [...PRESET_COLORS, "#5b3fa8", "#000000", "#ffffff"]) {
    const { h, s, l } = hexToHsl(c);
    assert.equal(hslToHex(h, s, l), c, c);
  }
  assert.equal(hslToHex(0, 1, 0.5), "#ff0000");
  assert.equal(hslToHex(120, 1, 0.5), "#00ff00");
  const right = wheelPoint(64, 0, 64);
  assert.equal(right.h, 0);
  assert.equal(right.s, 1);
  const down = wheelPoint(0, 32, 64); // screen y grows down: clockwise
  assert.equal(down.h, 90);
  assert.equal(down.s, 0.5);
  assert.equal(wheelPoint(-200, 0, 64).s, 1, "clamped to the rim");
  const pos = wheelPosition(90, 0.5, 64);
  assert.ok(Math.abs(pos.x) < 1e-9 && Math.abs(pos.y - 32) < 1e-9);
});

test("color selection: a preset sets the color at once", () => {
  let s = { color: "#64748b", custom: null };
  s = colorPicker(s, { type: "preset", color: "#F472B6" });
  assert.deepEqual(s, { color: "#f472b6", custom: null });
});

test("color selection: the wheel, lightness and hex only apply on Apply, and Cancel keeps the old color", () => {
  let s = colorPicker({ color: "#8b8fff", custom: null }, { type: "open" });
  assert.equal(s.custom.hex, "#8b8fff");
  s = colorPicker(s, { type: "wheel", h: 0, s: 1 });
  s = colorPicker(s, { type: "lightness", l: 0.5 });
  assert.equal(s.custom.hex, "#ff0000");
  assert.equal(s.color, "#8b8fff", "not applied yet");
  assert.equal(colorPicker(s, { type: "cancel" }).color, "#8b8fff");
  assert.deepEqual(colorPicker(s, { type: "apply" }), { color: "#ff0000", custom: null });

  s = colorPicker(s, { type: "hex", text: "#5B3FA8" });
  assert.equal(s.custom.error, undefined);
  assert.equal(hslToHex(s.custom.h, s.custom.s, s.custom.l), "#5b3fa8", "the wheel follows the typed hex");
  assert.equal(colorPicker(s, { type: "apply" }).color, "#5b3fa8");

  s = colorPicker(s, { type: "hex", text: "#5b3f" });
  assert.match(s.custom.error, /#rrggbb/);
  const applied = colorPicker(s, { type: "apply" });
  assert.notEqual(applied.custom, null, "an invalid hex can't be applied");
  assert.equal(applied.color, "#8b8fff");
});

test("low-contrast colors on the chat background get a warning and a readable suggestion", () => {
  assert.equal(contrastWarning("#4fd1c5"), null);
  const w = contrastWarning("#5b3fa8");
  assert.ok(w);
  assert.match(w.ratio, /^\d\.\d:1$/);
  assert.ok(contrastRatio(w.suggestion, CHAT_BG) >= MIN_CONTRAST);
  const light = contrastWarning("#f0bf4c", "#ffffff");
  assert.ok(light && contrastRatio(light.suggestion, "#ffffff") >= MIN_CONTRAST, "darkens on a light background");
  for (const c of PRESET_COLORS) assert.equal(contrastWarning(c), null, `preset ${c} reads on the dark theme`);
});

test("new personas get the first unused preset", () => {
  assert.equal(unusedPreset([]), PRESET_COLORS[0]);
  assert.equal(unusedPreset([{ color: "#8B8FFF" }, { color: "#4fd1c5" }]), PRESET_COLORS[2]);
  assert.equal(unusedPreset(PRESET_COLORS.map((color) => ({ color }))), PRESET_COLORS[0]);
});

test("avatar: client checks catch the wrong type, too large and empty files", () => {
  assert.equal(checkAvatarFile({ name: "me.png", type: "image/png", size: 1000 }), null);
  assert.equal(checkAvatarFile({ name: "me.jpg", type: "image/jpeg", size: AVATAR_MAX_BYTES }), null);
  assert.equal(checkAvatarFile({ name: "odd", type: "", size: 10 }), null, "unknown type: the server decides");
  assert.deepEqual(checkAvatarFile({ name: "selfie.heic", type: "image/heic", size: 10 }), { title: "selfie.heic: type not supported.", hint: "Use PNG, JPG or WebP." });
  assert.deepEqual(checkAvatarFile({ name: "a.svg", type: "image/svg+xml", size: 10 })?.hint, "Use PNG, JPG or WebP.");
  assert.deepEqual(checkAvatarFile({ name: "a.gif", type: "image/gif", size: 10 })?.hint, "Use PNG, JPG or WebP.");
  assert.deepEqual(checkAvatarFile({ name: "banner.png", type: "image/png", size: 6.4 * 1_048_576 }), {
    title: "banner.png is 6.4 MB.",
    hint: "The limit is 1 MB. Try a smaller image.",
  });
  assert.match(checkAvatarFile({ name: "e.png", type: "image/png", size: 0 }).title, /couldn't read/);
});

test("avatar: the server's 413, 415 and 422 become inline errors", () => {
  const f = { name: "x.png", size: 2 * 1_048_576 };
  assert.equal(avatarUploadError({ status: 413 }, f).title, "x.png is 2.0 MB.");
  assert.equal(avatarUploadError({ status: 415 }, f).title, "x.png: type not supported.");
  assert.match(avatarUploadError({ status: 422 }, f).title, /couldn't read/);
  const other = avatarUploadError(Object.assign(new Error("boom"), { status: 500 }), f);
  assert.deepEqual(other, { title: "x.png didn't upload.", hint: "boom" });
});

test("errors: 409 is a conflict, 422 with a field maps to that input", () => {
  assert.equal(isConflict({ status: 409 }), true);
  assert.equal(isConflict({ status: 422 }), false);
  assert.equal(isConflict(null), false);
  assert.deepEqual(fieldError({ status: 422, field: "name", message: "Already used" }), { field: "name", message: "Already used" });
  assert.equal(fieldError({ status: 422 }), null);
  assert.equal(fieldError({ status: 500, field: "name" }), null);
});

test("set default: the list shows the default first, then A–Z", () => {
  const list = [p("s", "sage"), p("a", "Atlas", { is_default: true }), p("n", "Nova"), p("r", "Rex")];
  assert.deepEqual(sortPersonas(list).map((x) => x.id), ["a", "n", "r", "s"]);
  const next = withDefault(list, "r");
  assert.deepEqual(next.filter((x) => x.is_default).map((x) => x.id), ["r"], "exactly one default");
  assert.equal(next[2], list[2], "untouched rows keep their identity");
  assert.deepEqual(sortPersonas(next).map((x) => x.id), ["r", "a", "n", "s"]);
  assert.deepEqual(withDefault(list, "a"), list);
});

test("deleting the default: the preselected successor matches the server's promotion rule", () => {
  const list = [
    p("a", "Atlas", { is_default: true, last_used_at: "2026-11-07T10:00:00Z" }),
    p("n", "Nova", { last_used_at: "2026-11-06T10:00:00Z" }),
    p("s", "Sage", { last_used_at: "2026-11-06T10:00:00Z", created_at: "2026-11-03T00:00:00Z" }),
    p("r", "Rex"),
  ];
  assert.equal(promotionCandidate(list, "a")?.id, "s", "ties go to the most recently created");
  assert.equal(promotionCandidate([p("r", "Rex"), p("x", "X", { created_at: "2026-11-05T00:00:00Z" })], "q")?.id, "x");
  assert.equal(promotionCandidate([list[0]], "a"), undefined, "none left: the built-in Assistant");
});

test("initials and delete copy", () => {
  assert.equal(personaInitial("nova"), "N");
  assert.equal(personaInitial("  "), "?");
  assert.equal(personaInitial("émile"), "É");
  assert.equal(deleteConsequence(0), "It has no conversations.");
  assert.equal(deleteConsequence(1), "Its conversation is kept and continues with the built-in Assistant.");
  assert.equal(deleteConsequence(12), "Its 12 conversations are kept and continue with the built-in Assistant.");
});

test("repo URLs: https, ssh and scp-style pass; credentials and junk fail", () => {
  for (const u of ["https://github.com/acme/app", "https://github.com/acme/app.git", "ssh://git@github.com/acme/app.git", "git@github.com:acme/app.git"]) {
    assert.deepEqual(checkRepoUrl(` ${u} `), { ok: true, url: u }, u);
  }
  const cred = checkRepoUrl("https://user:token@github.com/acme/app");
  assert.equal(cred.ok, false);
  assert.match(cred.message, /Connections & secrets/);
  assert.equal(checkRepoUrl("https://token@github.com/acme/app").ok, false);
  assert.equal(checkRepoUrl("http://github.com/acme/app").ok, false);
  assert.equal(checkRepoUrl("github.com/acme/app").ok, false);
  assert.equal(checkRepoUrl("https://github.com/acme").ok, false);
  assert.equal(checkRepoUrl("").ok, false);
});

test("repo short name is owner/repo", () => {
  assert.equal(repoShortName("https://github.com/acme/app.git"), "acme/app");
  assert.equal(repoShortName("git@github.com:acme/app.git"), "acme/app");
  assert.equal(repoShortName("ssh://git@gitlab.com/group/sub/app"), "sub/app");
  assert.equal(repoShortName("nonsense"), "nonsense");
});

test("the repository chip lists the persona's repos, then saved ones, once each", () => {
  const saved = [{ url: "https://github.com/acme/app.git" }, { url: "https://github.com/acme/web" }, { url: "git@github.com:acme/api.git" }];
  assert.equal(sameRepo("https://github.com/Acme/App.git", "git@github.com:acme/app"), true);
  const c = repositoryChoices(["https://github.com/acme/app", "https://github.com/acme/app"], saved, "https://github.com/acme/app");
  assert.deepEqual(c.persona.map((r) => r.name), ["acme/app"]);
  assert.deepEqual(c.saved.map((r) => r.name), ["acme/web", "acme/api"]);
  assert.equal(c.current, undefined);
  // A repo picked with /repo that neither list holds still shows, first.
  assert.equal(repositoryChoices([], saved, "https://github.com/other/thing").current?.name, "other/thing");
  // The one in use is lifted out of the saved list rather than shown twice.
  const lifted = repositoryChoices([], saved, "git@github.com:acme/web.git");
  assert.equal(lifted.current?.name, "acme/web");
  assert.deepEqual(lifted.saved.map((r) => r.name), ["acme/app", "acme/api"]);
  assert.equal(repositoryChoices([], Array.from({ length: 30 }, (_, i) => ({ url: `https://github.com/a/r${i}` }))).saved.length, 8);
  assert.deepEqual(repositoryChoices([], []), { current: undefined, persona: [], saved: [] });
});

test("adding repos dedupes and caps at 10", () => {
  assert.deepEqual(addRepo([], "https://github.com/a/b"), { repos: ["https://github.com/a/b"] });
  assert.ok("error" in addRepo(["https://github.com/a/b"], "https://GitHub.com/a/b"));
  assert.ok("error" in addRepo([], "nope"));
  const ten = Array.from({ length: 10 }, (_, i) => `https://github.com/a/r${i}`);
  assert.ok("error" in addRepo(ten, "https://github.com/a/more"));
});

test("image and ability helpers", () => {
  assert.equal(isImageType("image/png"), true);
  assert.equal(isImageType("application/pdf"), false);
  assert.equal(codesInRepos({ capabilities: { code: true }, repos: ["x"] }), true);
  assert.equal(codesInRepos({ capabilities: { code: true }, repos: [] }), false);
  assert.equal(codesInRepos(undefined), false);
});

test("mentionQuery finds the @mention being typed", async () => {
  const { mentionQuery } = await import("./persona-ui.ts");
  assert.deepEqual(mentionQuery("hi @Be", 6), { start: 3, query: "Be" });
  assert.deepEqual(mentionQuery("@", 1), { start: 0, query: "" });
  assert.deepEqual(mentionQuery("(@Writing co", 12), { start: 1, query: "Writing co" });
  assert.equal(mentionQuery("me@host", 7), null);
  assert.equal(mentionQuery("@Bea\nnext", 9), null);
  assert.equal(mentionQuery("@ spaced", 8), null);
  assert.equal(mentionQuery("no mention", 10), null);
});

test("mentionSuggestions prefers names that start with the query", async () => {
  const { mentionSuggestions } = await import("./persona-ui.ts");
  const ps = [{ name: "Writing coach" }, { name: "Bea" }, { name: "Coach Carter" }];
  assert.deepEqual(mentionSuggestions(ps, "coa").map((p) => p.name), ["Coach Carter", "Writing coach"]);
  assert.deepEqual(mentionSuggestions(ps, "").map((p) => p.name), ["Writing coach", "Bea", "Coach Carter"]);
  assert.deepEqual(mentionSuggestions(ps, "zz"), []);
});

test("insertMention replaces the query and moves the caret", async () => {
  const { insertMention } = await import("./persona-ui.ts");
  assert.deepEqual(insertMention("hey @be what", 4, 7, "Bea"), { text: "hey @Bea  what", caret: 9 });
});

test("the most used persona has the most chats, ties go to the most recently used, else the fallback", () => {
  const p = (id, chat_count, last_used_at) => ({ id, chat_count, ...(last_used_at ? { last_used_at } : {}) });
  assert.equal(mostUsedPersona([p("maya", 3, "2026-11-09T00:00:00Z"), p("bot", 7, "2026-11-01T00:00:00Z")], "maya"), "bot");
  assert.equal(mostUsedPersona([p("bot", 4, "2026-11-01T00:00:00Z"), p("coach", 4, "2026-11-05T00:00:00Z"), p("maya", 4)], "maya"), "coach");
  assert.equal(mostUsedPersona([p("bot", 0), p("coach", 0)], "maya"), "maya");
  assert.equal(mostUsedPersona(undefined, undefined), undefined);
});

test("cssHex reads a theme color in the minifier's short form too", () => {
  assert.equal(cssHex(" #fff"), "#ffffff");
  assert.equal(cssHex("#FCFBFE"), "#fcfbfe");
  assert.equal(cssHex("rgb(0 0 0)"), null);
  assert.equal(cssHex(""), null);
});
