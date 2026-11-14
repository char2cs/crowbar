export interface ParsedExcalidrawScene {
  elements: unknown[]
  appState: Record<string, unknown>
}

/** Structural only: `elements` is the signal. `appState` is optional because real
 *  `.excalidraw` files often omit it and `restore()` fills in defaults. */
function parseExcalidrawSceneUncached(raw: string): ParsedExcalidrawScene | null {
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

/** Keyed by source text so the scene reference is stable across unrelated
 *  re-renders: `ExcalidrawPreview` restarts its SVG export when it changes. */
const sceneCache = new Map<string, ParsedExcalidrawScene | null>()

export function parseExcalidrawScene(raw: string): ParsedExcalidrawScene | null {
  const cached = sceneCache.get(raw)
  if (cached !== undefined) return cached
  const scene = parseExcalidrawSceneUncached(raw)
  sceneCache.set(raw, scene)
  return scene
}

/** Lets `ExcalidrawPreview` reserve its footprint before the lazy engine loads, so
 *  the transcript doesn't resize a second time. `null` when nothing is measurable. */
export function computeSceneAspectRatio(elements: unknown[]): number | null {
  let minX = Infinity
  let minY = Infinity
  let maxX = -Infinity
  let maxY = -Infinity
  for (const el of elements) {
    if (typeof el !== 'object' || el === null) continue
    const { x, y, width, height } = el as Record<string, unknown>
    if (
      typeof x !== 'number' ||
      typeof y !== 'number' ||
      typeof width !== 'number' ||
      typeof height !== 'number'
    )
      continue
    minX = Math.min(minX, x)
    minY = Math.min(minY, y)
    maxX = Math.max(maxX, x + width)
    maxY = Math.max(maxY, y + height)
  }
  const sceneWidth = maxX - minX
  const sceneHeight = maxY - minY
  if (!(sceneWidth > 0) || !(sceneHeight > 0)) return null
  return sceneHeight / sceneWidth
}
