/** Fills the page with the surface develop paints under the pane: the editor view's `bg-pane-background` for a file or review, the pane chrome (on Linux the dark sidebar wash) for the rest, since a pane page has no container around it that paints the content area. */
export function paintPaneSurface(body: HTMLElement, typeId: string): void {
  const editorView = ['core.editor', 'core.review'].includes(typeId)
  body.style.backgroundColor = editorView ? 'var(--pane-background)' : 'var(--pane-chrome-bg)'
}
