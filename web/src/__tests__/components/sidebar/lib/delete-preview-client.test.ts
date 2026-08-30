import { afterEach, describe, expect, it, vi } from 'vitest'
import { fetchDeletePreview } from '@/components/sidebar/lib/delete-preview-client'

function envelope(data: unknown) {
  return { ok: true, status: 200, json: async () => ({ success: true, data }) }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('fetchDeletePreview', () => {
  it('GETs the project/repo-scoped delete-preview route and unwraps the envelope', async () => {
    const fetchMock = vi.fn().mockResolvedValue(envelope({ chatCount: 3, fileCount: 6 }))
    vi.stubGlobal('fetch', fetchMock)

    const preview = await fetchDeletePreview('p1', 'r1', 'ws-a')

    expect(preview).toEqual({ chatCount: 3, fileCount: 6 })
    expect(fetchMock).toHaveBeenCalledExactlyOnceWith(
      expect.stringContaining('/v0/projects/p1/repos/r1/chats/ws-a/delete-preview'),
      expect.anything(),
    )
  })
})
