import { describe, it, expect } from 'vitest'
import { computeFileTreeSearchHits } from '@/features/file-explorer/lib/visible-file-tree-rows'
import type { FileEntry } from '@/features/file-system/types/app'

// Regression: the file-tree search was wired to filterFileTreeForFffHits with a
// hardcoded [] (the query was never used), so every search showed "No matching
// files". computeFileTreeSearchHits is the missing piece that turns the query
// into hits.
const tree: FileEntry[] = [
  { name: 'README.md', path: 'README.md', isDir: false },
  { name: 'Makefile', path: 'Makefile', isDir: false },
  {
    name: 'src',
    path: 'src',
    isDir: true,
    children: [
      { name: 'index.ts', path: 'src/index.ts', isDir: false },
      { name: 'readme-helper.ts', path: 'src/readme-helper.ts', isDir: false },
    ],
  },
]

describe('computeFileTreeSearchHits', () => {
  it('matches a file by case-insensitive substring', () => {
    expect(computeFileTreeSearchHits(tree, 'readme').map((h) => h.path)).toEqual([
      'README.md',
      'src/readme-helper.ts',
    ])
  })

  it('matches a directory by its own name (filename match, not full path)', () => {
    // 'index.ts' / 'readme-helper.ts' don't contain "src" in their NAME, so only
    // the folder matches. (The UI then shows the folder with its children.)
    expect(computeFileTreeSearchHits(tree, 'src').map((h) => h.path)).toEqual(['src'])
  })

  it('returns no hits for an empty/whitespace query', () => {
    expect(computeFileTreeSearchHits(tree, '')).toEqual([])
    expect(computeFileTreeSearchHits(tree, '   ')).toEqual([])
  })

  it('returns no hits when nothing matches', () => {
    expect(computeFileTreeSearchHits(tree, 'zzz-nope')).toEqual([])
  })

  it('skips in-progress inline-edit placeholder nodes', () => {
    const withPlaceholder: FileEntry[] = [
      { name: '', path: 'src/', isDir: false, isNewItem: true, isEditing: true },
      { name: 'real.ts', path: 'real.ts', isDir: false },
    ]
    expect(computeFileTreeSearchHits(withPlaceholder, 'r').map((h) => h.path)).toEqual(['real.ts'])
  })
})
