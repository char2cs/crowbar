import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/lib/api', () => ({ apiFetch: vi.fn() }))

import { apiFetch } from '@/lib/api'
import {
  fetchFileTree,
  filesWsEndpoint,
  findNode,
  mergeChildren,
  toAppFile,
} from '@/features/files/lib/file-tree-api'
import { setWorkspaceScope } from '@/lib/workspace-scope'

// §3: file routes are hierarchical; register the scope for the test wsIds.
beforeEach(() => {
  setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-1' })
  setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-9' })
})

afterEach(() => vi.clearAllMocks())

describe('toAppFile', () => {
  it('maps a directory with no children to children: undefined (not loaded)', () => {
    const f = toAppFile({ name: 'src', path: 'src', type: 'directory' })
    expect(f).toMatchObject({ name: 'src', path: 'src', isDir: true, isFile: false })
    expect(f.children).toBeUndefined()
  })

  it('maps a file and carries gitStatus', () => {
    const f = toAppFile({ name: 'a.ts', path: 'src/a.ts', type: 'file', gitStatus: 'modified' })
    expect(f).toMatchObject({ isDir: false, isFile: true, gitStatus: 'modified' })
  })
})

describe('fetchFileTree', () => {
  it('hits the workspace-scoped root route with no path', async () => {
    ;(apiFetch as ReturnType<typeof vi.fn>).mockResolvedValue([
      { name: 'README.md', path: 'README.md', type: 'file' },
    ])
    const tree = await fetchFileTree('ws-1')
    expect(apiFetch).toHaveBeenCalledWith('/v0/projects/p1/repos/r1/workspaces/ws-1/files/tree')
    expect(tree[0].name).toBe('README.md')
  })

  it('encodes the path query for a subdirectory', async () => {
    ;(apiFetch as ReturnType<typeof vi.fn>).mockResolvedValue([])
    await fetchFileTree('ws-1', 'src/utils')
    expect(apiFetch).toHaveBeenCalledWith(
      '/v0/projects/p1/repos/r1/workspaces/ws-1/files/tree?path=src%2Futils',
    )
  })
})

describe('filesWsEndpoint', () => {
  it('builds the wsId-scoped files topic', () => {
    expect(filesWsEndpoint('ws-9')).toBe('/v0/projects/p1/repos/r1/workspaces/ws-9/files/ws')
  })
})

describe('findNode / mergeChildren', () => {
  const tree = [
    { name: 'src', path: 'src', isDir: true, children: undefined },
    { name: 'README.md', path: 'README.md', isDir: false },
  ]

  it('finds a top-level node', () => {
    expect(findNode(tree, 'src')?.name).toBe('src')
    expect(findNode(tree, 'missing')).toBeNull()
  })

  it('merges children into the target directory immutably', () => {
    const child = { name: 'a.ts', path: 'src/a.ts', isDir: false }
    const next = mergeChildren(tree, 'src', [child])
    expect(findNode(next, 'src')?.children).toEqual([child])
    expect(tree[0].children).toBeUndefined()
    expect(next[1]).toBe(tree[1])
  })

  it('finds and merges into a nested directory', () => {
    const nested = mergeChildren(tree, 'src', [
      { name: 'utils', path: 'src/utils', isDir: true, children: undefined },
    ])
    const deeper = mergeChildren(nested, 'src/utils', [
      { name: 'noop.ts', path: 'src/utils/noop.ts', isDir: false },
    ])
    expect(findNode(deeper, 'src/utils')?.children?.[0].name).toBe('noop.ts')
  })
})
