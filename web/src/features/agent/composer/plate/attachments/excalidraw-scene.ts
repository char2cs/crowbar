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

/**
 * The scene's own height/width ratio, straight from its elements' bounding
 * boxes — no Excalidraw engine involved, just the same `x`/`y`/`width`/
 * `height` every element type carries in the file format itself.
 *
 * Exists so `ExcalidrawPreview` can reserve its real footprint BEFORE the
 * live render (`@excalidraw/excalidraw`, dynamically imported) resolves,
 * instead of starting at a short text-line placeholder and jumping to full
 * size once it does. That jump is a second, LATE resize on top of the one
 * a settling message row already costs — turn-end already glides the
 * transcript into place once; an agent-authored diagram made it do so
 * again a beat later, and the spring's own settle motion (follow-scroll.ts)
 * is long and visibly springy BY DESIGN now, so two of those landing close
 * together read as the transcript fighting itself rather than one
 * continuous catch-up. Reported live as "the scroll bugs out."
 *
 * Returns null for anything this can't measure (no elements, or elements
 * with no numeric bounds) — the caller falls back to today's un-reserved
 * placeholder rather than reserving a wrong or zero-height box.
 */
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
