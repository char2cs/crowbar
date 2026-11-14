import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/features/files/lib/file-tree-api', () => ({ fetchFileTree: vi.fn() }))

import { fetchFileTree } from '@/features/files/lib/file-tree-api'
import {
  breadcrumbSegmentPath,
  loadDirectoryEntries,
} from '@/features/editor/components/toolbar/breadcrumb-directory'

const mockFetchFileTree = fetchFileTree as ReturnType<typeof vi.fn>

beforeEach(() => vi.clearAllMocks())

describe('breadcrumbSegmentPath', () => {
  it('builds a workspace-relative path from the segments alone', () => {
    expect(breadcrumbSegmentPath(['web', 'src', 'a.ts'], 1)).toBe('web/src')
  })

  it('returns the first segment for index 0', () => {
    expect(breadcrumbSegmentPath(['web', 'src', 'a.ts'], 0)).toBe('web')
  })

  it('returns the full path for the last segment', () => {
    expect(breadcrumbSegmentPath(['web', 'src', 'a.ts'], 2)).toBe('web/src/a.ts')
  })
})

describe('loadDirectoryEntries', () => {
  it('lists the given workspace, directories first then name-insensitive', async () => {
    mockFetchFileTree.mockResolvedValue([
      { name: 'b.ts', path: 'src/b.ts', isDir: false },
      { name: 'Utils', path: 'src/Utils', isDir: true },
      { name: 'a.ts', path: 'src/a.ts', isDir: false },
    ])

    const entries = await loadDirectoryEntries('ws-1', 'src')

    expect(mockFetchFileTree).toHaveBeenCalledWith('ws-1', 'src')
    expect(entries).toEqual([
      { name: 'Utils', path: 'src/Utils', isDir: true, children: undefined },
      { name: 'a.ts', path: 'src/a.ts', isDir: false, children: undefined },
      { name: 'b.ts', path: 'src/b.ts', isDir: false, children: undefined },
    ])
  })

  it('propagates a read failure so the caller can report it', async () => {
    mockFetchFileTree.mockRejectedValue(new Error('boom'))
    await expect(loadDirectoryEntries('ws-1', 'src')).rejects.toThrow('boom')
  })
})
