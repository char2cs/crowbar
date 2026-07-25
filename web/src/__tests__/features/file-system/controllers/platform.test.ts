import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// filesBase() resolves the active workspace through the registry, which drags in
// the whole editor/Monaco store graph. Stub it — the unit under test only needs
// an id.
vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getActiveWorkspaceId: () => 'ws-1',
}))

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return { ...actual, apiFetch: vi.fn() }
})

import { apiFetch, ApiError } from '@/lib/api'
import {
  exists,
  readDirectory,
  readFile,
  readWorkspaceFile,
} from '@/features/file-system/controllers/platform'
import { setWorkspaceScope, __resetWorkspaceScopesForTest } from '@/lib/workspace-scope'

const mockFetch = apiFetch as ReturnType<typeof vi.fn>

const WS_BASE = '/v0/projects/p1/repos/r1/workspaces/ws-1'

beforeEach(() => {
  vi.clearAllMocks()
  setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-1' })
})

afterEach(() => {
  __resetWorkspaceScopesForTest()
})

describe('readDirectory', () => {
  it('lists a subdirectory through the workspace-scoped tree route', async () => {
    mockFetch.mockResolvedValue([
      { name: 'utils', path: 'src/utils', type: 'directory' },
      { name: 'a.ts', path: 'src/a.ts', type: 'file' },
    ])

    const entries = await readDirectory('src')

    expect(mockFetch).toHaveBeenCalledWith(`${WS_BASE}/files/tree?path=src`)
    expect(entries).toEqual([
      { name: 'utils', path: 'src/utils', isDirectory: true, is_dir: true, isFile: false },
      { name: 'a.ts', path: 'src/a.ts', isDirectory: false, is_dir: false, isFile: true },
    ])
  })

  it('lists the workspace root when the path is empty', async () => {
    mockFetch.mockResolvedValue([])
    await readDirectory('')
    expect(mockFetch).toHaveBeenCalledWith(`${WS_BASE}/files/tree`)
  })

  it('percent-encodes the directory path', async () => {
    mockFetch.mockResolvedValue([])
    await readDirectory('src/my dir')
    expect(mockFetch).toHaveBeenCalledWith(`${WS_BASE}/files/tree?path=src%2Fmy%20dir`)
  })

  it('propagates a daemon failure instead of answering with an empty listing', async () => {
    // The whole point: "Open All Files" wraps this in a try/catch that falls back
    // to the already-loaded tree. A resolved [] skips that fallback AND the
    // caller's length check, so the feature silently does nothing.
    mockFetch.mockRejectedValue(new ApiError('boom', 500))
    await expect(readDirectory('src')).rejects.toThrow('boom')
  })
})

describe('exists', () => {
  it('is true when the parent listing contains the path', async () => {
    mockFetch.mockResolvedValue([
      { name: 'a.md', path: 'docs/a.md', type: 'file' },
      { name: 'b.md', path: 'docs/b.md', type: 'file' },
    ])

    await expect(exists('docs/b.md')).resolves.toBe(true)
    expect(mockFetch).toHaveBeenCalledWith(`${WS_BASE}/files/tree?path=docs`)
  })

  it('is false when the parent listing does not contain the path', async () => {
    mockFetch.mockResolvedValue([{ name: 'a.md', path: 'docs/a.md', type: 'file' }])
    await expect(exists('docs/missing.md')).resolves.toBe(false)
  })

  it('resolves a workspace-root path against the root listing', async () => {
    mockFetch.mockResolvedValue([{ name: 'README.md', path: 'README.md', type: 'file' }])
    await expect(exists('README.md')).resolves.toBe(true)
    expect(mockFetch).toHaveBeenCalledWith(`${WS_BASE}/files/tree`)
  })

  it('is false when the parent directory itself is missing (404)', async () => {
    mockFetch.mockRejectedValue(new ApiError('not found', 404))
    await expect(exists('nope/x.md')).resolves.toBe(false)
  })

  it('propagates a non-404 failure rather than reporting "does not exist"', async () => {
    mockFetch.mockRejectedValue(new ApiError('boom', 500))
    await expect(exists('docs/a.md')).rejects.toThrow('boom')
  })
})

describe('base64 content decoding', () => {
  it('readWorkspaceFile decodes a base64 payload', async () => {
    mockFetch.mockResolvedValue({ content: btoa('\u0000binary\u00ff'), encoding: 'base64' })
    await expect(readWorkspaceFile('ws-1', 'a.bin')).resolves.toBe('\u0000binary\u00ff')
  })

  it('readWorkspaceFile returns UTF-8 payloads untouched', async () => {
    mockFetch.mockResolvedValue({ content: 'plain text' })
    await expect(readWorkspaceFile('ws-1', 'a.ts')).resolves.toBe('plain text')
  })

  it('readFile decodes a base64 payload', async () => {
    mockFetch.mockResolvedValue({ content: btoa('\u0000binary\u00ff'), encoding: 'base64' })
    await expect(readFile('a.bin')).resolves.toBe('\u0000binary\u00ff')
  })
})
