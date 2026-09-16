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
  writeWorkspaceFile,
} from '@/features/file-system/controllers/platform'
import {
  setWorkspaceScope,
  recordWorkspaceScope,
  __resetWorkspaceScopesForTest,
} from '@/lib/workspace-scope'

const mockFetch = apiFetch as ReturnType<typeof vi.fn>

// Files are addressed through the chat that holds the worktree, so every URL
// below is named by a chat id and never by a workspace id.
const CHAT_BASE = '/v0/chats/chat-1'

beforeEach(() => {
  vi.clearAllMocks()
  setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-1', owningChatId: 'chat-1' })
})

afterEach(() => {
  __resetWorkspaceScopesForTest()
})

describe('readDirectory', () => {
  it('lists a subdirectory through the chat-scoped tree route', async () => {
    mockFetch.mockResolvedValue([
      { name: 'utils', path: 'src/utils', type: 'directory' },
      { name: 'a.ts', path: 'src/a.ts', type: 'file' },
    ])

    const entries = await readDirectory('src')

    expect(mockFetch).toHaveBeenCalledWith(`${CHAT_BASE}/files/tree?path=src`)
    expect(entries).toEqual([
      { name: 'utils', path: 'src/utils', isDirectory: true, is_dir: true, isFile: false },
      { name: 'a.ts', path: 'src/a.ts', isDirectory: false, is_dir: false, isFile: true },
    ])
  })

  it('lists the workspace root when the path is empty', async () => {
    mockFetch.mockResolvedValue([])
    await readDirectory('')
    expect(mockFetch).toHaveBeenCalledWith(`${CHAT_BASE}/files/tree`)
  })

  it('percent-encodes the directory path', async () => {
    mockFetch.mockResolvedValue([])
    await readDirectory('src/my dir')
    expect(mockFetch).toHaveBeenCalledWith(`${CHAT_BASE}/files/tree?path=src%2Fmy%20dir`)
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
    expect(mockFetch).toHaveBeenCalledWith(`${CHAT_BASE}/files/tree?path=docs`)
  })

  it('is false when the parent listing does not contain the path', async () => {
    mockFetch.mockResolvedValue([{ name: 'a.md', path: 'docs/a.md', type: 'file' }])
    await expect(exists('docs/missing.md')).resolves.toBe(false)
  })

  it('resolves a workspace-root path against the root listing', async () => {
    mockFetch.mockResolvedValue([{ name: 'README.md', path: 'README.md', type: 'file' }])
    await expect(exists('README.md')).resolves.toBe(true)
    expect(mockFetch).toHaveBeenCalledWith(`${CHAT_BASE}/files/tree`)
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

describe('writeWorkspaceFile', () => {
  // Task 26 / Critical 1: writeFile's implicit filesBase() -> the ACTIVE
  // workspace is exactly the hazard writeWorkspaceFile exists to avoid for a
  // caller that already knows which workspace a write belongs to (Save/
  // Save All/autosave, LSP-rename — see editor-app-store.test.ts for the
  // full integration proof). getActiveWorkspaceId is mocked to a fixed
  // 'ws-1' at the top of this file; ws-2 is deliberately a DIFFERENT,
  // non-active workspace here, to prove the explicit wsId argument wins.
  it('writes to the EXPLICIT workspace passed, ignoring the active one', async () => {
    recordWorkspaceScope({ projectId: 'p1', repoId: 'r2', wsId: 'ws-2', owningChatId: 'chat-2' })
    mockFetch.mockResolvedValue({})

    await writeWorkspaceFile('ws-2', 'a.ts', 'content for ws-2')

    expect(mockFetch).toHaveBeenCalledWith(
      '/v0/chats/chat-2/files/content',
      expect.objectContaining({
        method: 'PUT',
        body: JSON.stringify({ path: 'a.ts', content: 'content for ws-2' }),
      }),
    )
    // Never touched the active workspace's own URL.
    expect(mockFetch).not.toHaveBeenCalledWith(
      expect.stringContaining('/chats/chat-1/'),
      expect.anything(),
    )
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
