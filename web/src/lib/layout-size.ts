/**
 * An element's height in its own CSS pixels. `getBoundingClientRect` reports
 * visual pixels, which CSS `zoom` on an ancestor scales; feeding that into
 * layout math inside the zoomed subtree overshoots by the zoom factor.
 */
export function layoutHeight(el: Element): number {
  const rect = el.getBoundingClientRect()
  const zoom = el instanceof HTMLElement && el.offsetWidth > 0 ? rect.width / el.offsetWidth : 1
  return zoom > 0 ? rect.height / zoom : rect.height
}
