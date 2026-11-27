import { currentPlatform } from '@/utils/platform'

/** Fills the page with the surface develop paints under the pane: the editor view's `bg-pane-background` for a file or review, the pane chrome (on Linux the dark sidebar wash) for the rest. A Mac's shell already paints that wash behind its see-through page, so a second one there would darken it. */
export function paintPaneSurface(
  body: HTMLElement,
  typeId: string,
  platform: typeof currentPlatform = currentPlatform,
): void {
  const editorView = ['core.editor', 'core.review'].includes(typeId)
  const chrome = platform === 'macos' ? 'transparent' : 'var(--pane-chrome-bg)'
  body.style.backgroundColor = editorView ? 'var(--pane-background)' : chrome
}
