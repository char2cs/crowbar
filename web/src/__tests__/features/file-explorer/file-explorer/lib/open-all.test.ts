import { describe, expect, it } from 'vitest'
import {
  collectLoadedFilesInDirectory,
  collectLocalFilesInDirectory,
  getPathBaseName,
} from '@/features/file-explorer/file-explorer/lib/open-all'
import type { FileEntry } from '@/features/file-system/types/app'

const tree: FileEntry[] = [
  {
    name: 'src',
    path: 'src',
    isDir: true,
    children: [
      { name: 'a.ts', path: 'src/a.ts' },
      {
        name: 'lib',
        path: 'src/lib',
        isDir: true,
        children: [{ name: 'b.ts', path: 'src/lib/b.ts' }],
      },
    ],
  },
  { name: 'README.md', path: 'README.md' },
]

describe('collectLoadedFilesInDirectory', () => {
  it('walks the whole tree for the root (absolute root path or empty)', () => {
    const all = ['src/a.ts', 'src/lib/b.ts', 'README.md']
    expect(collectLoadedFilesInDirectory(tree, '/repos/r1', '/repos/r1')).toEqual(all)
    expect(collectLoadedFilesInDirectory(tree, '', '/repos/r1')).toEqual(all)
  })

  it('walks one directory, and returns nothing for a file or unknown path', () => {
    expect(collectLoadedFilesInDirectory(tree, 'src/lib', '/r')).toEqual(['src/lib/b.ts'])
    expect(collectLoadedFilesInDirectory(tree, 'README.md', '/r')).toEqual([])
    expect(collectLoadedFilesInDirectory(tree, 'nope', '/r')).toEqual([])
  })
})

describe('collectLocalFilesInDirectory', () => {
  const disk: Record<string, Array<{ path: string; is_dir: boolean }>> = {
    src: [
      { path: 'src/a.ts', is_dir: false },
      { path: 'src/.DS_Store', is_dir: false },
      { path: 'src/.cache', is_dir: true },
      { path: 'src/lib', is_dir: true },
    ],
    'src/lib': [{ path: 'src/lib/b.ts', is_dir: false }],
    'src/.cache': [{ path: 'src/.cache/x', is_dir: false }],
  }
  const read = async (path: string) => disk[path] ?? []

  it('recurses into directories, skipping .DS_Store and what isVisible rejects', async () => {
    const files = await collectLocalFilesInDirectory(
      'src',
      read,
      (_p, name) => !name.startsWith('.'),
    )
    expect(files.sort()).toEqual(['src/a.ts', 'src/lib/b.ts'])
  })

  it('propagates read failures so the caller can fall back to the loaded tree', async () => {
    await expect(
      collectLocalFilesInDirectory(
        'src',
        () => Promise.reject(new Error('down')),
        () => true,
      ),
    ).rejects.toThrow('down')
  })
})

describe('getPathBaseName', () => {
  it('takes the last segment, ignoring trailing separators', () => {
    expect(getPathBaseName('a/b/c.ts')).toBe('c.ts')
    expect(getPathBaseName('a\\b\\')).toBe('b')
    expect(getPathBaseName('/')).toBe('/')
  })
})
