import { describe, expect, it } from 'vitest'
import {
  buildGitFolderTree,
  collectNodeFiles,
  createFolderNode,
  normalizePathSegments,
  sortFilesByPath,
  sortFoldersByName,
} from '@/features/git/utils/build-git-folder-tree'
import type { GitFile } from '@/features/git/types/git-types'

const makeFile = (path: string, staged = false): GitFile => ({
  path,
  status: 'modified',
  staged,
})

describe('normalizePathSegments', () => {
  it('splits a unix path into segments', () => {
    expect(normalizePathSegments('src/sub/b.ts')).toEqual(['src', 'sub', 'b.ts'])
  })

  it('normalizes backslashes to forward slashes', () => {
    expect(normalizePathSegments('src\\sub\\b.ts')).toEqual(['src', 'sub', 'b.ts'])
  })

  it('strips empty segments from leading/trailing slashes', () => {
    expect(normalizePathSegments('/src/a.ts/')).toEqual(['src', 'a.ts'])
  })

  it('returns empty array for empty string', () => {
    expect(normalizePathSegments('')).toEqual([])
  })
})

describe('createFolderNode', () => {
  it('creates a node with correct shape', () => {
    const node = createFolderNode('src', 'src')
    expect(node.name).toBe('src')
    expect(node.fullPath).toBe('src')
    expect(node.folders).toBeInstanceOf(Map)
    expect(node.files).toEqual([])
  })
})

describe('buildGitFolderTree', () => {
  it('places root-level files at the root node', () => {
    const files = [makeFile('README.md')]
    const root = buildGitFolderTree(files)
    expect(root.files).toHaveLength(1)
    expect(root.files[0].path).toBe('README.md')
    expect(root.folders.size).toBe(0)
  })

  it('nests src/a.ts under src folder', () => {
    const files = [makeFile('src/a.ts')]
    const root = buildGitFolderTree(files)
    expect(root.folders.has('src')).toBe(true)
    const srcNode = root.folders.get('src')!
    expect(srcNode.files).toHaveLength(1)
    expect(srcNode.files[0].path).toBe('src/a.ts')
  })

  it('nests src/sub/b.ts under src -> sub', () => {
    const files = [makeFile('src/sub/b.ts')]
    const root = buildGitFolderTree(files)
    const srcNode = root.folders.get('src')!
    expect(srcNode.folders.has('sub')).toBe(true)
    const subNode = srcNode.folders.get('sub')!
    expect(subNode.files).toHaveLength(1)
    expect(subNode.files[0].path).toBe('src/sub/b.ts')
  })

  it('builds the full tree: src nests sub; README.md at root', () => {
    const files = [makeFile('src/a.ts'), makeFile('src/sub/b.ts'), makeFile('README.md')]
    const root = buildGitFolderTree(files)

    // Root-level file
    expect(root.files).toHaveLength(1)
    expect(root.files[0].path).toBe('README.md')

    // src folder
    expect(root.folders.has('src')).toBe(true)
    const srcNode = root.folders.get('src')!
    expect(srcNode.name).toBe('src')
    expect(srcNode.fullPath).toBe('src')
    expect(srcNode.files).toHaveLength(1)
    expect(srcNode.files[0].path).toBe('src/a.ts')

    // src/sub folder
    expect(srcNode.folders.has('sub')).toBe(true)
    const subNode = srcNode.folders.get('sub')!
    expect(subNode.name).toBe('sub')
    expect(subNode.fullPath).toBe('src/sub')
    expect(subNode.files).toHaveLength(1)
    expect(subNode.files[0].path).toBe('src/sub/b.ts')
  })

  it('returns empty root for empty input', () => {
    const root = buildGitFolderTree([])
    expect(root.files).toHaveLength(0)
    expect(root.folders.size).toBe(0)
  })

  it('skips files with empty paths', () => {
    const files = [makeFile(''), makeFile('a.ts')]
    const root = buildGitFolderTree(files)
    expect(root.files).toHaveLength(1)
    expect(root.files[0].path).toBe('a.ts')
  })

  it('shares the same folder node for sibling files', () => {
    const files = [makeFile('src/a.ts'), makeFile('src/b.ts')]
    const root = buildGitFolderTree(files)
    const srcNode = root.folders.get('src')!
    expect(srcNode.files).toHaveLength(2)
  })
})

describe('sortFoldersByName', () => {
  it('sorts folders alphabetically by name', () => {
    const folders = [
      createFolderNode('z-folder', 'z-folder'),
      createFolderNode('a-folder', 'a-folder'),
      createFolderNode('m-folder', 'm-folder'),
    ]
    const sorted = sortFoldersByName(folders)
    expect(sorted.map((f) => f.name)).toEqual(['a-folder', 'm-folder', 'z-folder'])
  })

  it('works with a Map iterator', () => {
    const files = [makeFile('z/a.ts'), makeFile('a/b.ts')]
    const root = buildGitFolderTree(files)
    const sorted = sortFoldersByName(root.folders.values())
    expect(sorted[0].name).toBe('a')
    expect(sorted[1].name).toBe('z')
  })
})

describe('sortFilesByPath', () => {
  it('sorts files by path', () => {
    const files = [makeFile('z.ts'), makeFile('a.ts'), makeFile('m.ts')]
    const sorted = sortFilesByPath(files)
    expect(sorted.map((f) => f.path)).toEqual(['a.ts', 'm.ts', 'z.ts'])
  })

  it('does not mutate the original array', () => {
    const files = [makeFile('b.ts'), makeFile('a.ts')]
    sortFilesByPath(files)
    expect(files[0].path).toBe('b.ts')
  })
})

describe('collectNodeFiles', () => {
  it('collects all files from nested tree', () => {
    const files = [makeFile('src/a.ts'), makeFile('src/sub/b.ts'), makeFile('README.md')]
    const root = buildGitFolderTree(files)

    // Collect from src node: should include src/a.ts and src/sub/b.ts
    const srcNode = root.folders.get('src')!
    const collected = collectNodeFiles(srcNode)
    expect(collected).toHaveLength(2)
    expect(collected.map((f) => f.path).sort()).toEqual(['src/a.ts', 'src/sub/b.ts'])
  })

  it('collects all files from root', () => {
    const files = [makeFile('src/a.ts'), makeFile('src/sub/b.ts'), makeFile('README.md')]
    const root = buildGitFolderTree(files)
    const collected = collectNodeFiles(root)
    expect(collected).toHaveLength(3)
  })

  it('returns empty array for leaf node with no files', () => {
    const node = createFolderNode('empty', 'empty')
    expect(collectNodeFiles(node)).toEqual([])
  })
})
