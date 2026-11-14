// The person's last saved Excalidraw scene per chat, client-side and best-effort, so reopening
// the editor (the "+" entry or Edit on a diagram) resumes it instead of starting blank.

const KEY_PREFIX = 'crowbar:agent-excalidraw-design:v1:'

// A single scene, not an accumulating queue — one budget per entry is enough.
export const MAX_EXCALIDRAW_DESIGN_BYTES = 512 * 1024

export function excalidrawDesignStorageKey(wsId: string, chatId: string): string {
  return `${KEY_PREFIX}${encodeURIComponent(wsId)}:${encodeURIComponent(chatId)}`
}

/** The raw scene JSON string last saved for this chat, or null if none was,
 *  or the stored value can't be read. Shape validation is `parseExcalidrawScene`'s
 *  job, not this module's — a corrupt value just fails there like any other. */
export function loadExcalidrawDesign(wsId: string, chatId: string): string | null {
  try {
    return localStorage.getItem(excalidrawDesignStorageKey(wsId, chatId))
  } catch {
    return null
  }
}

/** Replaces this chat's saved design. Returns false (without writing) when
 *  the scene exceeds the size cap or storage itself is unavailable/full —
 *  the caller keeps working with its in-memory copy either way. */
export function saveExcalidrawDesign(wsId: string, chatId: string, sceneJson: string): boolean {
  if (new TextEncoder().encode(sceneJson).byteLength > MAX_EXCALIDRAW_DESIGN_BYTES) return false
  try {
    localStorage.setItem(excalidrawDesignStorageKey(wsId, chatId), sceneJson)
    return true
  } catch {
    return false
  }
}
