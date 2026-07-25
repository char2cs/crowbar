import { describe, expect, it } from 'vitest'
import { findFileInTree } from '@/features/file-system/controllers/file-tree-utils'
import type { AppFile } from '@/features/file-system/types/app'

// findFileInTree returned null unconditionally. Two live callers depend on it:
// the "Open All Files" fallback walk (which then reported an empty directory)
// and the inline create-file/folder duplicate-name guard (which therefore never
// warned before overwriting an existing name).
const tree: AppFile[] = [
  {
    name: 'src',
    path: 'src',
    isDir: true,
    children: [
      { name: 'a.ts', path: 'src/a.ts', isDir: false },
      {
        name: 'utils',
        path: 'src/utils',
        isDir: true,
        children: [{ name: 'noop.ts', path: 'src/utils/noop.ts', isDir: false }],
      },
    ],
  },
  { name: 'README.md', path: 'README.md', isDir: false },
]

describe('findFileInTree', () => {
  it('finds a top-level node', () => {
    expect(findFileInTree(tree, 'README.md')?.name).toBe('README.md')
  })

  it('finds a nested directory', () => {
    expect(findFileInTree(tree, 'src/utils')?.isDir).toBe(true)
  })

  it('finds a deeply nested file', () => {
    expect(findFileInTree(tree, 'src/utils/noop.ts')?.name).toBe('noop.ts')
  })

  it('returns null for a path that is not in the tree', () => {
    expect(findFileInTree(tree, 'src/missing.ts')).toBeNull()
  })

  it('returns null for an empty tree', () => {
    expect(findFileInTree([], 'src')).toBeNull()
  })
})
