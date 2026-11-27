const ATTRIBUTE = 'data-editor-selecting'

let depth = 0

export function isSelectionDragging(): boolean {
  return depth > 0
}

export function beginSelectionDrag(): void {
  depth++
  if (depth === 1 && typeof document !== 'undefined') {
    document.documentElement.setAttribute(ATTRIBUTE, '1')
  }
}

export function endSelectionDrag(): void {
  if (depth === 0) return
  depth--
  if (depth === 0 && typeof document !== 'undefined') {
    document.documentElement.removeAttribute(ATTRIBUTE)
  }
}

export function releaseSelectionDrag(held: boolean): void {
  if (held) endSelectionDrag()
}
