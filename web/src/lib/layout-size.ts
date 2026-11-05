// An element's height in its own CSS pixels. getBoundingClientRect reports visual pixels, scaled
// by an ancestor's CSS `zoom`, which overshoots layout math inside the zoomed subtree.
export function layoutHeight(el: Element): number {
  const rect = el.getBoundingClientRect()
  const zoom = el instanceof HTMLElement && el.offsetWidth > 0 ? rect.width / el.offsetWidth : 1
  return zoom > 0 ? rect.height / zoom : rect.height
}
