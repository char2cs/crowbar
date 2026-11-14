import { describe, expect, it } from 'vitest'
import type { PaneContent } from '@/features/panes/types/pane-content'
import {
  getAncestorDirectoryPaths,
  getExplorerTargetPath,
} from '@/features/file-explorer/utils/file-explorer-tree-utils'

describe('getAncestorDirectoryPaths', () => {
  it('returns parent directories from root to leaf parent', () => {
    expect(
      getAncestorDirectoryPaths(
        '/workspace/src/features/file-explorer/components/file-explorer-tree.tsx',
        '/workspace',
      ),
    ).toEqual([
      '/workspace',
      '/workspace/src',
      '/workspace/src/features',
      '/workspace/src/features/file-explorer',
      '/workspace/src/features/file-explorer/components',
    ])
  })
})

describe('getExplorerTargetPath', () => {
  it('uses the source file for preview buffers', () => {
    const buffer = {
      id: 'preview',
      type: 'markdownPreview',
      path: '/workspace/README.md:preview',
      name: 'README.md (Preview)',
      isPinned: false,
      isPreview: false,
      content: '# Test',
      source: { authority: 'workspace', workspaceId: 'w1', path: '/workspace/README.md' },
      workspaceId: 'w1',
    } satisfies PaneContent

    expect(getExplorerTargetPath(buffer)).toBe('/workspace/README.md')
  })

  it('ignores a document outside every workspace, and its preview', () => {
    const editor = {
      id: 'host',
      type: 'editor',
      authority: 'host',
      path: '/tmp/notes.md',
      name: 'notes.md',
      isDirty: false,
      isVirtual: false,
      isPinned: false,
      isPreview: false,
      workspaceId: '',
    } satisfies PaneContent
    const preview = {
      id: 'host-preview',
      type: 'markdownPreview',
      path: '/tmp/notes.md:preview',
      name: 'notes.md (Preview)',
      content: '',
      source: { authority: 'host', path: '/tmp/notes.md' },
      isPinned: false,
      isPreview: false,
      workspaceId: '',
    } satisfies PaneContent

    expect(getExplorerTargetPath(editor)).toBeUndefined()
    expect(getExplorerTargetPath(preview)).toBeUndefined()
  })

  it('ignores non-file buffers', () => {
    const buffer = {
      id: 'term',
      type: 'terminal',
      path: 'terminal://t1',
      name: 'Terminal 1',
      sessionId: 't1',
      isPinned: false,
      isPreview: false,
      workspaceId: 'w1',
    } satisfies PaneContent

    expect(getExplorerTargetPath(buffer)).toBeUndefined()
  })
})
