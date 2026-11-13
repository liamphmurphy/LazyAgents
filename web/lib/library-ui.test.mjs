import test from "node:test";
import assert from "node:assert/strict";
import { LIBRARY_UPLOAD_MAX_BYTES, checkLibraryUpload, referenceBadge, referenceCaption, referenceChoices } from "./library-ui.ts";

test("checkLibraryUpload takes images, audio and files up to 25 MB, and turns executables away", () => {
  assert.equal(checkLibraryUpload({ name: "me.jpg", type: "image/jpeg", size: 1000 }), null);
  assert.equal(checkLibraryUpload({ name: "me.webp", type: "image/webp", size: LIBRARY_UPLOAD_MAX_BYTES }), null);
  // Anything else is kept as a file; only executables and disk images are refused.
  for (const name of ["notes.pdf", "x.svg", "render.py", "bundle.zip", "q3.csv", "model.xlsx", "README"]) assert.equal(checkLibraryUpload({ name, type: "", size: 10 }), null, name);
  for (const name of ["setup.exe", "Installer.MSI", "disk.iso", "tool.dmg", "app.jar"]) assert.match(checkLibraryUpload({ name, type: "", size: 10 }), /executables and disk images/, name);
  // A picture that isn't one of the image types is a mistake worth catching early.
  assert.match(checkLibraryUpload({ name: "me.png", type: "image/bmp", size: 10 }), /isn't a PNG, JPEG, GIF or WebP image/);
  // Audio is picked by name: browsers report no type for some of these.
  for (const name of ["memo.mp3", "memo.WAV", "memo.ogg", "memo.m4a"]) assert.equal(checkLibraryUpload({ name, type: "", size: 1000 }), null, name);
  assert.equal(checkLibraryUpload({ name: "memo.flac", type: "audio/flac", size: 10 }), null, "other audio is kept as a file");
  assert.match(checkLibraryUpload({ name: "memo.wav", type: "audio/wav", size: LIBRARY_UPLOAD_MAX_BYTES + 1 }), /25 MB/);
  assert.match(checkLibraryUpload({ name: "memo.mp3", type: "audio/mpeg", size: 0 }), /empty/);
  assert.match(checkLibraryUpload({ name: "e.png", type: "image/png", size: 0 }), /empty/);
  assert.match(checkLibraryUpload({ name: "big.png", type: "image/png", size: LIBRARY_UPLOAD_MAX_BYTES + 1 }), /25 MB/);
});

test("referenceBadge names whom a reference shows", () => {
  assert.equal(referenceBadge({}), null);
  assert.equal(referenceBadge({ reference: "self" }), "You");
  assert.equal(referenceBadge({ reference: "persona", reference_persona_id: "p1", reference_persona_name: "Gwen" }), "Gwen");
  // A persona reference whose persona is gone isn't one.
  assert.equal(referenceBadge({ reference: "persona" }), null);
  assert.equal(referenceCaption({ reference: "self" }), "Your reference");
  assert.equal(referenceCaption({ reference: "persona", reference_persona_id: "p1", reference_persona_name: "Gwen" }), "Gwen's reference");
  assert.equal(referenceCaption({}), null);
});

test("referenceChoices offers the person, then each persona, minus the current role", () => {
  const personas = [{ id: "p1", name: "Gwen" }, { id: "p2", name: "Ada" }];
  assert.deepEqual(
    referenceChoices({}, personas).map((c) => c.label),
    ["Use as my reference", "Use as Gwen's reference", "Use as Ada's reference"],
  );
  assert.deepEqual(referenceChoices({ reference: "self" }, personas).map((c) => c.key), ["persona:p1", "persona:p2"]);
  const choices = referenceChoices({ reference: "persona", reference_persona_id: "p1" }, personas);
  assert.deepEqual(choices.map((c) => c.key), ["self", "persona:p2"]);
  assert.deepEqual(choices[1], { key: "persona:p2", label: "Use as Ada's reference", role: "persona", personaId: "p2" });
});
