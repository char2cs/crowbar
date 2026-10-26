import { isMarkdownPath } from '@/features/editor/markdown/plate/is-markdown-path'

// The editors are separate chunks, fetched when a pane first draws one. The pane and whoever
// opens a file both load them through here, so the module system hands each the same chunk.
export const loadEditorSurface = () => import('@/features/editor/components/editor-surface')
export const loadMarkdownEditorPane = () =>
  import('@/features/editor/markdown/plate/markdown-editor-pane')

/** Resolves once the editor that draws `path` is loaded, so a tab opened after it is never blank. */
export function preloadEditorFor(path: string): Promise<unknown> {
  return isMarkdownPath(path) ? loadMarkdownEditorPane() : loadEditorSurface()
}
