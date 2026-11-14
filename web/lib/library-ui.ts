// Pure helpers for the Library screen: uploads and reference images.
// No React or fetch here so node --test can cover them.

type ItemLike = {
  reference?: "persona" | "self";
  reference_persona_id?: string;
  reference_persona_name?: string;
};

type PersonaLike = { id: string; name: string };

/** Matches the server's Library upload limit and the types it keeps. */
export const LIBRARY_UPLOAD_MAX_BYTES = 25 * 1024 * 1024;
export const LIBRARY_UPLOAD_TYPES = ["image/png", "image/jpeg", "image/gif", "image/webp"] as const;
/** What the server refuses outright: executables, installers and disk images. It checks the bytes too. */
export const LIBRARY_DENIED_EXTENSIONS = [".exe", ".dll", ".msi", ".scr", ".com", ".pif", ".bat", ".cmd", ".vbs", ".lnk", ".app", ".apk", ".ipa", ".dmg", ".iso", ".img", ".vhd", ".vhdx", ".jar"] as const;

/** Why a picked file can't be uploaded, or null. The server checks the bytes too. */
export function checkLibraryUpload(file: { name: string; type: string; size: number }): string | null {
  const name = file.name.toLowerCase();
  if (LIBRARY_DENIED_EXTENSIONS.some((ext) => name.endsWith(ext))) return `${file.name} isn't kept: executables and disk images aren't allowed.`;
  // A picture must be one of the image types; anything else is kept as audio or a file.
  if (/\.(png|jpe?g|gif|webp)$/.test(name) && !(LIBRARY_UPLOAD_TYPES as readonly string[]).includes(file.type)) return `${file.name} isn't a PNG, JPEG, GIF or WebP image.`;
  if (file.size <= 0) return `${file.name} is empty.`;
  if (file.size > LIBRARY_UPLOAD_MAX_BYTES) return `${file.name} is over 25 MB.`;
  return null;
}

/** The badge for a current reference image: "You" or the persona's name; null when it isn't one. */
export function referenceBadge(item: ItemLike): string | null {
  if (item.reference === "self") return "You";
  if (item.reference === "persona" && item.reference_persona_id) return item.reference_persona_name ?? "Persona";
  return null;
}

/** "Your reference" or "Gwen's reference" for the open image; null when it isn't one. */
export function referenceCaption(item: ItemLike): string | null {
  const who = referenceBadge(item);
  if (!who) return null;
  return who === "You" ? "Your reference" : `${who}'s reference`;
}

export type ReferenceChoice = { key: string; label: string; role: "persona" | "self"; personaId?: string };

/**
 * The "Use as reference" menu for an item: the person first, then each
 * persona, leaving out what the item already stands for.
 */
export function referenceChoices(item: ItemLike, personas: PersonaLike[]): ReferenceChoice[] {
  const out: ReferenceChoice[] = [];
  if (item.reference !== "self") out.push({ key: "self", label: "Use as my reference", role: "self" });
  for (const p of personas) {
    if (item.reference === "persona" && item.reference_persona_id === p.id) continue;
    out.push({ key: `persona:${p.id}`, label: `Use as ${p.name}'s reference`, role: "persona", personaId: p.id });
  }
  return out;
}
