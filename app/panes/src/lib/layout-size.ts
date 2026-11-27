export function layoutHeight(el: Element): number {
  const rect = el.getBoundingClientRect()
  const zoom = el instanceof HTMLElement && el.offsetWidth > 0 ? rect.width / el.offsetWidth : 1
  return zoom > 0 ? rect.height / zoom : rect.height
}
