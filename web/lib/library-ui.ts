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
export const LIBRARY_AUDIO_EXTENSIONS = [".mp3", ".wav", ".ogg", ".oga", ".m4a"] as const;
export const LIBRARY_UPLOAD_ACCEPT = ".png,.jpg,.jpeg,.gif,.webp,image/png,image/jpeg,image/gif,image/webp,.mp3,.wav,.ogg,.oga,.m4a,audio/mpeg,audio/wav,audio/ogg,audio/mp4";

/** Whether a picked file is audio, by its name: browsers disagree on the types of .wav and .m4a. */
export function isAudioFile(file: { name: string }): boolean {
  const name = file.name.toLowerCase();
  return LIBRARY_AUDIO_EXTENSIONS.some((ext) => name.endsWith(ext));
}

/** Why a picked file can't be uploaded, or null. The server checks the bytes too. */
export function checkLibraryUpload(file: { name: string; type: string; size: number }): string | null {
  if (!isAudioFile(file) && !(LIBRARY_UPLOAD_TYPES as readonly string[]).includes(file.type)) return `${file.name} isn't a PNG, JPEG, GIF or WebP image, or an MP3, WAV, OGG or M4A file.`;
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
