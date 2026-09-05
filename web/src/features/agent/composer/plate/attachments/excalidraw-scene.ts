export interface ParsedExcalidrawScene {
  elements: unknown[]
  appState: Record<string, unknown>
}

/** Structural check only — not the real Excalidraw type (that library isn't
 *  a dependency of this read-only preview renderer). Just enough to reject a
 *  fenced block that merely mentions "excalidraw" in ordinary prose, or valid
 *  JSON that isn't a scene, before treating it as a real diagram. */
export function parseExcalidrawScene(raw: string): ParsedExcalidrawScene | null {
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return null
  }
  if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) return null
  const obj = parsed as Record<string, unknown>
  if (!Array.isArray(obj.elements)) return null
  if (typeof obj.appState !== 'object' || obj.appState === null || Array.isArray(obj.appState)) return null
  return { elements: obj.elements, appState: obj.appState as Record<string, unknown> }
}
