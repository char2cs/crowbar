/**
 * Pure sizing math for the imperative pixel sash splitter.
 *
 * The authoritative layout stores split ratios as `[firstPct, secondPct]`
 * percentages summing to 100. During a drag we work in pixels for crisp,
 * reflow-free movement, then convert back to percentages on pointer-up.
 */

/**
 * Clamp the first pane's pixel size so neither pane drops below `minPx`.
 * If the container is too small to honour both minimums, collapse to the
 * midpoint so the divider stays centered rather than jumping to an edge.
 */
export function clampFirstPx(firstPx: number, containerPx: number, minPx: number): number {
  if (containerPx <= 0) return 0
  if (containerPx < minPx * 2) return containerPx / 2
  const min = minPx
  const max = containerPx - minPx
  if (firstPx < min) return min
  if (firstPx > max) return max
  return firstPx
}

/**
 * Convert a dragged first-pane pixel size into `[firstPct, secondPct]`
 * percentages summing to exactly 100, with each pane clamped to `minPx`.
 */
export function pxToPercents(
  containerPx: number,
  firstPx: number,
  minPx: number,
): [number, number] {
  if (containerPx <= 0) return [50, 50]
  const clamped = clampFirstPx(firstPx, containerPx, minPx)
  const firstPct = (clamped / containerPx) * 100
  const secondPct = 100 - firstPct
  return [firstPct, secondPct]
}

/**
 * Convert `[firstPct, secondPct]` percentages into pixel sizes that sum to
 * exactly `containerPx` (the second pane absorbs any rounding remainder).
 */
export function percentsToPx(containerPx: number, sizes: [number, number]): [number, number] {
  const total = sizes[0] + sizes[1]
  const firstPct = total > 0 ? sizes[0] / total : 0.5
  const firstPx = Math.round(containerPx * firstPct)
  const secondPx = containerPx - firstPx
  return [firstPx, secondPx]
}
