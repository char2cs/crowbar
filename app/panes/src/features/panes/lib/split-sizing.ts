function clampFirstPx(firstPx: number, containerPx: number, minPx: number): number {
  if (containerPx <= 0) return 0
  if (containerPx < minPx * 2) return containerPx / 2
  const min = minPx
  const max = containerPx - minPx
  if (firstPx < min) return min
  if (firstPx > max) return max
  return firstPx
}

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

export function percentsToPx(containerPx: number, sizes: [number, number]): [number, number] {
  const total = sizes[0] + sizes[1]
  const firstPct = total > 0 ? sizes[0] / total : 0.5
  const firstPx = Math.round(containerPx * firstPct)
  const secondPx = containerPx - firstPx
  return [firstPx, secondPx]
}
