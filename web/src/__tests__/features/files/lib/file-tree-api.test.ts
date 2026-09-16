import { describe, it, expect, vi, beforeEach } from 'vitest'

// The file-explorer context menu / inline edit were dead because the file-system
// store's mutation slots were never wired. These call the real daemon endpoints
// (POST/PATCH/DELETE .../files). This locks the request shapes.
const { apiFetch } = vi.hoisted(() => ({ apiFetch: vi.fn() }))

vi.mock('@/lib/api', () => ({ apiFetch: (...a: unknown[]) => apiFetch(...a) }))
vi.mock('@/lib/workspace-scope-url', () => ({
  filesBaseForWorkspace: (wsId: string) => `/v0/chats/chat-for-${wsId}/files`,
}))

import {
  copyFileNode,
  createFileNode,
  renameFileNode,
  deleteFileNode,
  writeFileContent,
} from '@/features/files/lib/file-tree-api'

beforeEach(() => {
  vi.clearAllMocks()
  apiFetch.mockResolvedValue(undefined)
})

describe('file-tree-api mutations', () => {
  it('createFileNode POSTs {path, type:"file"}', async () => {
    await createFileNode('ws1', 'src/new.ts', 'file')
    expect(apiFetch).toHaveBeenCalledWith('/v0/chats/chat-for-ws1/files', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: 'src/new.ts', type: 'file' }),
    })
  })

  it('createFileNode POSTs type:"dir" for folders', async () => {
    await createFileNode('ws1', 'src/sub', 'dir')
    expect(apiFetch.mock.calls[0][1].body).toBe(JSON.stringify({ path: 'src/sub', type: 'dir' }))
  })

  it('renameFileNode PATCHes {path, newPath}', async () => {
    await renameFileNode('ws1', 'a.ts', 'b.ts')
    expect(apiFetch).toHaveBeenCalledWith('/v0/chats/chat-for-ws1/files', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: 'a.ts', newPath: 'b.ts' }),
    })
  })

  it('deleteFileNode DELETEs {path}', async () => {
    await deleteFileNode('ws1', 'gone.ts')
    expect(apiFetch).toHaveBeenCalledWith('/v0/chats/chat-for-ws1/files', {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: 'gone.ts' }),
    })
  })

  it('writeFileContent PUTs {path, content} as UTF-8 by default (no encoding field)', async () => {
    await writeFileContent('ws1', 'a.ts', 'hi')
    expect(apiFetch).toHaveBeenCalledWith('/v0/chats/chat-for-ws1/files/content', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: 'a.ts', content: 'hi' }),
    })
  })

  it('writeFileContent includes encoding:"base64" for byte-faithful binary writes', async () => {
    await writeFileContent('ws1', 'img.png', 'iVBORw==', 'base64')
    expect(apiFetch.mock.calls[0][1].body).toBe(
      JSON.stringify({ path: 'img.png', content: 'iVBORw==', encoding: 'base64' }),
    )
  })
})

// Duplicate goes through the daemon's server-side copy verb in ONE call —
// byte-faithful and recursive on the server. The old client-side composition
// (GET content → POST create → PUT content) silently corrupted binary files by
// writing the base64 read back as text; this locks the single-call shape.
describe('copyFileNode', () => {
  it('POSTs {sourcePath, destPath} to the copy verb in a single call', async () => {
    await copyFileNode('ws1', 'img.png', 'img copy.png')

    expect(apiFetch).toHaveBeenCalledTimes(1)
    expect(apiFetch).toHaveBeenCalledWith('/v0/chats/chat-for-ws1/files/copy', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ sourcePath: 'img.png', destPath: 'img copy.png' }),
    })
  })
})
