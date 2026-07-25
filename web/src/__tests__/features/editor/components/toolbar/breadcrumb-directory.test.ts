import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/features/file-system/controllers/platform', () => ({ readDirectory: vi.fn() }))

import { readDirectory } from '@/features/file-system/controllers/platform'
import {
  breadcrumbSegmentPath,
  loadDirectoryEntries,
} from '@/features/editor/components/toolbar/breadcrumb-directory'

const mockReadDirectory = readDirectory as ReturnType<typeof vi.fn>

beforeEach(() => vi.clearAllMocks())

describe('breadcrumbSegmentPath', () => {
  // rootFolderPath is the WORKSPACE ID, not a filesystem prefix (see
  // use-workspace-effects: `rootFolderPath: wsId`). Joining it onto the segments
  // produced `<wsId>/web/src`, which the daemon rejects as a non-existent path —
  // so the dropdown never opened even once readDirectory was real.
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
  it('maps daemon entries onto FileEntry, directories first then name-insensitive', async () => {
    mockReadDirectory.mockResolvedValue([
      { name: 'b.ts', path: 'src/b.ts', isDirectory: false, is_dir: false, isFile: true },
      { name: 'Utils', path: 'src/Utils', isDirectory: true, is_dir: true, isFile: false },
      { name: 'a.ts', path: 'src/a.ts', isDirectory: false, is_dir: false, isFile: true },
    ])

    const entries = await loadDirectoryEntries('src')

    expect(mockReadDirectory).toHaveBeenCalledWith('src')
    expect(entries).toEqual([
      { name: 'Utils', path: 'src/Utils', isDir: true, children: undefined },
      { name: 'a.ts', path: 'src/a.ts', isDir: false, children: undefined },
      { name: 'b.ts', path: 'src/b.ts', isDir: false, children: undefined },
    ])
  })

  it('propagates a read failure so the caller can report it', async () => {
    mockReadDirectory.mockRejectedValue(new Error('boom'))
    await expect(loadDirectoryEntries('src')).rejects.toThrow('boom')
  })
})
