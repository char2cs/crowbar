import { useEffect, useMemo } from 'react'
import { detectLanguageFromPath } from '@/features/editor/utils/language-detection'
import type { EditorContent } from '@/features/panes/types/pane-content'
import { useWorkspaceStore } from '@/features/workspace/stores/workspace-context'
import type { TokenEntry } from '@/features/panes/types/pane-content'
import { createDiffTokensForEditorContent, getDiffEditorPath } from '../utils/diff-editor-content'

interface UseDiffEditorBufferOptions {
  cacheKey: string
  content: string
  sourcePath?: string
  name: string
  pathOverride?: string
  languageOverride?: string
  tokens?: TokenEntry[]
  /**
   * Skip registering the buffer entirely (default true). Callers that only
   * need this buffer conditionally — e.g. the split-diff left/right editors,
   * which are pointless work while the view is unified — pass `false` to
   * avoid the setState + Monaco-model churn. Flipping back to `true` mid-session
   * registers the buffer on the next effect run (it's in the dependency array).
   */
  enabled?: boolean
}

export function useDiffEditorBuffer({
  cacheKey,
  content,
  sourcePath,
  name,
  pathOverride,
  languageOverride,
  tokens,
  enabled = true,
}: UseDiffEditorBufferOptions): string {
  const workspaceStore = useWorkspaceStore()
  const bufferId = useMemo(
    () => `diff_editor_${cacheKey.replace(/[^a-zA-Z0-9_]/g, '_')}`,
    [cacheKey],
  )
  const bufferPath = useMemo(
    () => pathOverride ?? getDiffEditorPath(sourcePath, cacheKey),
    [cacheKey, pathOverride, sourcePath],
  )

  useEffect(() => {
    if (!enabled) return

    const nextBuffer: EditorContent = {
      id: bufferId,
      type: 'editor',
      path: bufferPath,
      name,
      content,
      savedContent: content,
      isDirty: false,
      isVirtual: true,
      isPreview: false,
      isPinned: false,
      isActive: false,
      language: detectLanguageFromPath(bufferPath),
      languageOverride,
      tokens:
        tokens ?? (languageOverride === 'diff' ? createDiffTokensForEditorContent(content) : []),
    }

    workspaceStore.setState((state) => {
      const existingIndex = state.buffers.findIndex((buffer) => buffer.id === bufferId)
      if (existingIndex === -1) {
        return {
          ...state,
          buffers: [...state.buffers, nextBuffer],
        }
      }

      const nextBuffers = [...state.buffers]
      nextBuffers[existingIndex] = {
        ...nextBuffers[existingIndex],
        ...nextBuffer,
      }

      return {
        ...state,
        buffers: nextBuffers,
      }
    })

    return () => {
      workspaceStore.setState((state) => ({
        ...state,
        buffers: state.buffers.filter((buffer) => buffer.id !== bufferId),
      }))
    }
  }, [
    workspaceStore,
    bufferId,
    bufferPath,
    content,
    enabled,
    languageOverride,
    name,
    sourcePath,
    tokens,
  ])

  return bufferId
}
