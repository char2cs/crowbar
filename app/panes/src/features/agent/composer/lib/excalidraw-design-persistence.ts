const KEY_PREFIX = 'crowbar:agent-excalidraw-design:v1:'

const MAX_EXCALIDRAW_DESIGN_BYTES = 512 * 1024

function excalidrawDesignStorageKey(wsId: string, chatId: string): string {
  return `${KEY_PREFIX}${encodeURIComponent(wsId)}:${encodeURIComponent(chatId)}`
}

export function loadExcalidrawDesign(wsId: string, chatId: string): string | null {
  try {
    return localStorage.getItem(excalidrawDesignStorageKey(wsId, chatId))
  } catch {
    return null
  }
}

export function saveExcalidrawDesign(wsId: string, chatId: string, sceneJson: string): boolean {
  if (new TextEncoder().encode(sceneJson).byteLength > MAX_EXCALIDRAW_DESIGN_BYTES) return false
  try {
    localStorage.setItem(excalidrawDesignStorageKey(wsId, chatId), sceneJson)
    return true
  } catch {
    return false
  }
}
