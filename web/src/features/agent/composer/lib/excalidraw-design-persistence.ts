/**
 * The user's own Excalidraw design for a chat, kept client-side only (no
 * backend attachment): a scene the user saved persists here so the next
 * time they open the editor for this chat — the "+" entry point, or Edit on
 * a rendered diagram — they resume it instead of starting blank. Mirrors
 * prompt-queue-persistence.ts's boundary: tiny, versioned, best-effort.
 */

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
