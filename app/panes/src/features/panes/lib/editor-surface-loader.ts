import { isMarkdownPath } from '@/features/editor/markdown/plate/is-markdown-path'

export const loadEditorSurface = () => import('@/features/editor/components/editor-surface')
export const loadMarkdownEditorPane = () =>
  import('@/features/editor/markdown/plate/markdown-editor-pane')

export function preloadEditorFor(path: string): Promise<unknown> {
  return isMarkdownPath(path) ? loadMarkdownEditorPane() : loadEditorSurface()
}
