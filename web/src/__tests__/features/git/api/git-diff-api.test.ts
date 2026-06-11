import { afterEach, describe, expect, it, vi } from 'vitest'
import { getCommitDiff } from '@/features/git/api/git-diff-api'

function mockFetchEnvelope(data: unknown): ReturnType<typeof vi.fn> {
  const fetchMock = vi.fn(async () => ({
    ok: true,
    status: 200,
    json: async () => ({ success: true, data }),
  }))
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('getCommitDiff', () => {
  // BUG-018 regression: the frontend used to call /git/diff?commit=<sha>,
  // a parameter the backend ignores — it returned the *working tree* diff
  // instead of the commit's. The real route is /git/commit-diff?sha=<sha>.
  it('requests the workspace-scoped commit-diff route with ?sha=', async () => {
    const fetchMock = mockFetchEnvelope({ files: [], totalFiles: 0 })

    await getCommitDiff('ws-1', 'deadbee')

    const url = fetchMock.mock.calls[0][0] as unknown as string
    expect(url).toContain('/v0/workspaces/ws-1/git/commit-diff')
    expect(url).toContain('sha=deadbee')
  })

  it('unwraps the MultiFileDiff envelope into the per-file diff array', async () => {
    const files = [
      { file_path: 'a.ts', lines: [{ line_type: 'added', content: 'x' }] },
      { file_path: 'b.ts', lines: [] },
    ]
    mockFetchEnvelope({ files, totalFiles: 2, totalAdditions: 1, totalDeletions: 0 })

    const diffs = await getCommitDiff('ws-2', 'cafe123')

    expect(diffs).toHaveLength(2)
    expect(diffs?.[0].file_path).toBe('a.ts')
  })
})
