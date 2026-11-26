export type PaneDropZone = 'left' | 'right' | 'top' | 'bottom' | 'center' | null

export function getPaneDropZoneFromRect(
  point: { x: number; y: number },
  rect: Pick<DOMRect, 'left' | 'top' | 'width' | 'height'>,
): PaneDropZone {
  const x = point.x - rect.left
  const y = point.y - rect.top
  const nx = x / rect.width
  const ny = y / rect.height
  const threshold = 0.25

  if (nx < threshold && nx < ny && nx < 1 - ny) return 'left'
  if (nx > 1 - threshold && 1 - nx < ny && 1 - nx < 1 - ny) return 'right'
  if (ny < threshold) return 'top'
  if (ny > 1 - threshold) return 'bottom'
  return 'center'
}
