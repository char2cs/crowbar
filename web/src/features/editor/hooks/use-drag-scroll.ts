// The editor's drag-to-autoscroll hook (useDragScroll) was never wired to a
// component and has been removed; this query — still read by the editor state
// store — is therefore always false. Kept as a stable no-op rather than
// threading its removal through the store's call site.
export function isDragScrolling(): boolean {
  return false
}
