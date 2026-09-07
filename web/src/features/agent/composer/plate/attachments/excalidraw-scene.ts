export interface ParsedExcalidrawScene {
  elements: unknown[]
  appState: Record<string, unknown>
}

/** Structural check only — not the real Excalidraw type (that library isn't
 *  a dependency of this read-only preview renderer). Just enough to reject a
 *  fenced block that merely mentions "excalidraw" in ordinary prose, or valid
 *  JSON that isn't a scene, before treating it as a real diagram.
 *
 * `appState` is OPTIONAL: it holds view/style state (background color,
 * viewport, ...), never what's actually drawn, and `restore()` (the
 * renderer's own normalization step, excalidraw-preview.tsx) already fills
 * in sensible defaults for a missing one. The real `.excalidraw` file
 * format — what an agent asked to write "valid Excalidraw JSON" is drawing
 * on — routinely omits it entirely; requiring it here rejected exactly the
 * scenes this check exists to accept. `elements` alone, a real array,
 * remains the load-bearing signal against a fenced block that merely
 * mentions the word in prose. */
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
  if (obj.appState !== undefined) {
    if (typeof obj.appState !== 'object' || obj.appState === null || Array.isArray(obj.appState))
      return null
  }
  return { elements: obj.elements, appState: (obj.appState as Record<string, unknown>) ?? {} }
}
