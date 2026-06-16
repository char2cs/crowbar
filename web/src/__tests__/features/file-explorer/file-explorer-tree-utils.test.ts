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

  it('supports remote-style paths', () => {
    expect(
      getAncestorDirectoryPaths(
        'remote://server/workspace/src/file.ts',
        'remote://server/workspace',
      ),
    ).toEqual(['remote://server/workspace', 'remote://server/workspace/src'])
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
      isActive: true,
      content: '# Test',
      sourceFilePath: '/workspace/README.md',
    } satisfies PaneContent

    expect(getExplorerTargetPath(buffer)).toBe('/workspace/README.md')
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
      isActive: true,
    } satisfies PaneContent

    expect(getExplorerTargetPath(buffer)).toBeUndefined()
  })
})
