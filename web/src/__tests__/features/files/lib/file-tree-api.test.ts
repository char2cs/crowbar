import { describe, it, expect, vi, beforeEach } from 'vitest'

// The file-explorer context menu / inline edit were dead because the file-system
// store's mutation slots were never wired. These call the real daemon endpoints
// (POST/PATCH/DELETE .../files). This locks the request shapes.
const { apiFetch } = vi.hoisted(() => ({ apiFetch: vi.fn() }))

vi.mock('@/lib/api', () => ({ apiFetch: (...a: unknown[]) => apiFetch(...a) }))
vi.mock('@/lib/workspace-scope-url', () => ({
  workspaceBase: (wsId: string) => `/v0/ws/${wsId}`,
}))

import { createFileNode, renameFileNode, deleteFileNode } from '@/features/files/lib/file-tree-api'

beforeEach(() => {
  vi.clearAllMocks()
  apiFetch.mockResolvedValue(undefined)
})

describe('file-tree-api mutations', () => {
  it('createFileNode POSTs {path, type:"file"}', async () => {
    await createFileNode('ws1', 'src/new.ts', 'file')
    expect(apiFetch).toHaveBeenCalledWith('/v0/ws/ws1/files', {
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
    expect(apiFetch).toHaveBeenCalledWith('/v0/ws/ws1/files', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: 'a.ts', newPath: 'b.ts' }),
    })
  })

  it('deleteFileNode DELETEs {path}', async () => {
    await deleteFileNode('ws1', 'gone.ts')
    expect(apiFetch).toHaveBeenCalledWith('/v0/ws/ws1/files', {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: 'gone.ts' }),
    })
  })
})
