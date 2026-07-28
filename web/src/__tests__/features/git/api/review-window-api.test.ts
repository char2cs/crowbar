import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  getReviewOutline,
  getReviewPatch,
  searchReviewDiff,
  DIFF_TRUNCATED_HEADER,
} from '@/features/git/api/review-window-api'
import { ApiError } from '@/lib/api'
import { setWorkspaceScope } from '@/lib/workspace-scope'

// §3: workspace-scoped URLs are hierarchical; register scopes for the test wsIds.
beforeEach(() => {
  for (const wsId of ['ws-1', 'ws-2', 'ws-3']) {
    setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId })
  }
})

afterEach(() => {
  vi.unstubAllGlobals()
})

const WS_BASE = '/v0/projects/p1/repos/r1/workspaces'

function mockEnvelope(data: unknown): ReturnType<typeof vi.fn> {
  const fetchMock = vi.fn(async () => ({
    ok: true,
    status: 200,
    json: async () => ({ success: true, data }),
  }))
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

/** A real Response so headers behave exactly as the browser's do. */
function mockText(body: string, headers: Record<string, string> = {}): ReturnType<typeof vi.fn> {
  const fetchMock = vi.fn(
    async () =>
      new Response(body, {
        status: 200,
        headers: { 'Content-Type': 'text/plain; charset=utf-8', ...headers },
      }),
  )
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function urlOf(fetchMock: ReturnType<typeof vi.fn>, call = 0): string {
  return fetchMock.mock.calls[call][0] as string
}

describe('getReviewOutline', () => {
  it('GETs the workspace-scoped /review/outline route and returns the files', async () => {
    const fetchMock = mockEnvelope({
      files: [
        {
          path: 'src/a.ts',
          hunks: [{ oldStart: 1, oldLines: 4, newStart: 1, newLines: 9 }],
          isPartial: false,
          isBinary: false,
        },
      ],
    })

    const outline = await getReviewOutline({ wsId: 'ws-1' })

    expect(urlOf(fetchMock)).toContain(`${WS_BASE}/ws-1/review/outline`)
    expect(outline).toHaveLength(1)
    expect(outline[0].path).toBe('src/a.ts')
    expect(outline[0].hunks).toEqual([{ oldStart: 1, oldLines: 4, newStart: 1, newLines: 9 }])
    expect(outline[0].isPartial).toBe(false)
    expect(outline[0].isBinary).toBe(false)
  })

  it('carries oldPath for a rename', async () => {
    mockEnvelope({
      files: [{ path: 'new.ts', oldPath: 'old.ts', hunks: [], isPartial: false, isBinary: false }],
    })

    const outline = await getReviewOutline({ wsId: 'ws-1' })

    expect(outline[0].oldPath).toBe('old.ts')
  })

  // Go marshals a nil []HunkShape as `null`, which is exactly what a binary file
  // produces (git emits no @@ headers for one). Consumers map over hunks.
  it('normalises a null hunks array to [] (binary file)', async () => {
    mockEnvelope({
      files: [{ path: 'logo.png', hunks: null, isPartial: false, isBinary: true }],
    })

    const outline = await getReviewOutline({ wsId: 'ws-1' })

    expect(outline[0].hunks).toEqual([])
    expect(outline[0].isBinary).toBe(true)
  })

  it('returns [] when the payload carries no files', async () => {
    mockEnvelope({ files: null })

    await expect(getReviewOutline({ wsId: 'ws-1' })).resolves.toEqual([])
  })

  it('flags a partial outline so the caller does not treat the last hunk as the end of the file', async () => {
    mockEnvelope({
      files: [
        {
          path: 'monster.js',
          hunks: [{ oldStart: 1, oldLines: 0, newStart: 1, newLines: 420000 }],
          isPartial: true,
          isBinary: false,
        },
      ],
    })

    const outline = await getReviewOutline({ wsId: 'ws-1' })

    expect(outline[0].isPartial).toBe(true)
  })
})

describe('getReviewPatch', () => {
  it('GETs /review/patch with the path as a query parameter and returns the raw text', async () => {
    const patch = 'diff --git a/x.ts b/x.ts\n@@ -1 +1 @@\n-a\n+b\n'
    const fetchMock = mockText(patch)

    const result = await getReviewPatch({ wsId: 'ws-1' }, 'src/x.ts')

    const url = urlOf(fetchMock)
    expect(url).toContain(`${WS_BASE}/ws-1/review/patch`)
    expect(url).toContain('path=src%2Fx.ts')
    expect(result.patch).toBe(patch)
  })

  // text/plain, not the {success,data} envelope. A patch that happens to look
  // like JSON must come back byte-for-byte, never unwrapped.
  it('does not run the body through the JSON envelope', async () => {
    const patch = '{"success":true,"data":"not a patch"}'
    mockText(patch)

    await expect(getReviewPatch({ wsId: 'ws-1' }, 'a.json')).resolves.toEqual({
      patch,
      truncated: false,
    })
  })

  it('reports truncated:false when the response carries no truncation header', async () => {
    mockText('diff --git a/x b/x\n')

    const result = await getReviewPatch({ wsId: 'ws-1' }, 'x')

    expect(result.truncated).toBe(false)
  })

  // Truncation travels in a leading RESPONSE HEADER, deliberately not an HTTP
  // trailer: fetch() exposes no trailers, so a trailer could never be read here.
  it('reads truncation from the X-Crowbar-Diff-Truncated response header', async () => {
    expect(DIFF_TRUNCATED_HEADER).toBe('X-Crowbar-Diff-Truncated')
    mockText('diff --git a/big b/big\n', { [DIFF_TRUNCATED_HEADER]: 'true' })

    const result = await getReviewPatch({ wsId: 'ws-1' }, 'big.js', 20000)

    expect(result.truncated).toBe(true)
  })

  // A header-only patch under the cap is the fixture's 420k-line single-hunk
  // monster: nothing fits, so the body is scaffolding alone. It must read as
  // truncated, never as an empty file.
  it('reports a header-only capped patch as truncated rather than empty', async () => {
    const headerOnly =
      'diff --git a/monster.js b/monster.js\nindex 000..111 100644\n--- a/monster.js\n+++ b/monster.js\n'
    mockText(headerOnly, { [DIFF_TRUNCATED_HEADER]: 'true' })

    const result = await getReviewPatch({ wsId: 'ws-1' }, 'monster.js', 20000)

    expect(result).toEqual({ patch: headerOnly, truncated: true })
  })

  it('omits maxLines entirely when the caller does not ask for a cap', async () => {
    const fetchMock = mockText('')

    await getReviewPatch({ wsId: 'ws-1' }, 'x.ts')

    expect(urlOf(fetchMock)).not.toContain('maxLines')
  })

  it('sends the requested cap', async () => {
    const fetchMock = mockText('')

    await getReviewPatch({ wsId: 'ws-1' }, 'x.ts', 500)

    expect(urlOf(fetchMock)).toContain('maxLines=500')
  })

  // maxLines=0 is the engine's "unlimited" — the "show all" refetch. The server
  // streams it and reports nothing, because an uncapped patch cannot be truncated.
  it('sends maxLines=0 for an uncapped refetch and reports truncated:false', async () => {
    const fetchMock = mockText('diff --git a/big b/big\n')

    const result = await getReviewPatch({ wsId: 'ws-2' }, 'big.js', 0)

    expect(urlOf(fetchMock)).toContain('maxLines=0')
    expect(result.truncated).toBe(false)
  })

  it('encodes a path with spaces and slashes', async () => {
    const fetchMock = mockText('')

    await getReviewPatch({ wsId: 'ws-1' }, 'src/a b/c&d.ts')

    const url = urlOf(fetchMock)
    expect(url).toContain('path=src%2Fa+b%2Fc%26d.ts')
  })

  it('throws an ApiError carrying the daemon message on an error status', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({ success: false, error: 'path query parameter is required' }),
            {
              status: 400,
              headers: { 'Content-Type': 'application/json' },
            },
          ),
      ),
    )

    const err = await getReviewPatch({ wsId: 'ws-1' }, 'x.ts').catch((e: unknown) => e)

    expect(err).toBeInstanceOf(ApiError)
    expect((err as ApiError).status).toBe(400)
    expect((err as ApiError).message).toBe('path query parameter is required')
  })
})

describe('searchReviewDiff', () => {
  it('GETs /review/search with the query and returns hits plus truncation', async () => {
    const fetchMock = mockEnvelope({
      hits: [{ path: 'src/a.ts', side: 'new', lineNumber: 42, preview: 'const todo = 1' }],
      truncated: true,
    })

    const result = await searchReviewDiff({ wsId: 'ws-1' }, 'todo')

    const url = urlOf(fetchMock)
    expect(url).toContain(`${WS_BASE}/ws-1/review/search`)
    expect(url).toContain('q=todo')
    expect(result.hits).toEqual([
      { path: 'src/a.ts', side: 'new', lineNumber: 42, preview: 'const todo = 1' },
    ])
    expect(result.truncated).toBe(true)
  })

  it('returns [] when the daemon answers no hits', async () => {
    mockEnvelope({ hits: null, truncated: false })

    await expect(searchReviewDiff({ wsId: 'ws-1' }, 'zzz')).resolves.toEqual({
      hits: [],
      truncated: false,
    })
  })

  it('sends no option parameters when none are asked for', async () => {
    const fetchMock = mockEnvelope({ hits: [], truncated: false })

    await searchReviewDiff({ wsId: 'ws-1' }, 'todo')

    const url = urlOf(fetchMock)
    expect(url).not.toContain('regex=')
    expect(url).not.toContain('case=')
    expect(url).not.toContain('limit=')
  })

  // The backend's case-sensitivity parameter is named `case`, not `caseSensitive`.
  it('maps caseSensitive onto the wire parameter `case`', async () => {
    const fetchMock = mockEnvelope({ hits: [], truncated: false })

    await searchReviewDiff({ wsId: 'ws-1' }, 'Todo', { caseSensitive: true })

    const url = urlOf(fetchMock)
    expect(url).toContain('case=true')
    expect(url).not.toContain('caseSensitive')
  })

  it('sends regex and limit', async () => {
    const fetchMock = mockEnvelope({ hits: [], truncated: false })

    await searchReviewDiff({ wsId: 'ws-3' }, 'a.*b', { regex: true, limit: 50 })

    const url = urlOf(fetchMock)
    expect(url).toContain('regex=true')
    expect(url).toContain('limit=50')
    // `*` is a legal query character and stays unescaped; the pattern must reach
    // the daemon intact rather than mangled.
    expect(url).toContain('q=a.*b')
  })

  it('encodes the query rather than letting it break the URL', async () => {
    const fetchMock = mockEnvelope({ hits: [], truncated: false })

    await searchReviewDiff({ wsId: 'ws-1' }, 'a&b=c d')

    expect(urlOf(fetchMock)).toContain('q=a%26b%3Dc+d')
  })

  it('surfaces an invalid-regex 400 as an ApiError instead of crashing', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({
        ok: false,
        status: 400,
        statusText: 'Bad Request',
        json: async () => ({ success: false, error: 'error parsing regexp: missing closing )' }),
      })),
    )

    const err = await searchReviewDiff({ wsId: 'ws-1' }, 'a(b', { regex: true }).catch(
      (e: unknown) => e,
    )

    expect(err).toBeInstanceOf(ApiError)
    expect((err as ApiError).status).toBe(400)
    expect((err as ApiError).message).toContain('missing closing')
  })

  it('sends an empty q rather than skipping the request when the find box is empty', async () => {
    const fetchMock = mockEnvelope({ hits: [], truncated: false })

    await searchReviewDiff({ wsId: 'ws-1' }, '')

    expect(urlOf(fetchMock)).toContain('q=')
  })
})

// A commit scope is what lets the history surface reuse the review surface
// instead of keeping a second diff renderer alive for it. If the parameter
// silently failed to travel, every route would quietly answer the BRANCH — a
// plausible-looking diff of the wrong thing, which is the worst kind of wrong.
describe('commit scope', () => {
  it('omits the sha entirely when the scope is the branch', async () => {
    const fetchMock = mockEnvelope({ files: [] })

    await getReviewOutline({ wsId: 'ws-1' })

    expect(urlOf(fetchMock)).not.toContain('sha=')
  })

  it('sends the sha on outline, patch and search', async () => {
    const outlineMock = mockEnvelope({ files: [] })
    await getReviewOutline({ wsId: 'ws-1', commit: 'abc1234' })
    expect(urlOf(outlineMock)).toContain('sha=abc1234')

    const patchMock = mockText('diff --git a/a.ts b/a.ts\n')
    await getReviewPatch({ wsId: 'ws-1', commit: 'abc1234' }, 'a.ts')
    expect(urlOf(patchMock)).toContain('sha=abc1234')
    expect(urlOf(patchMock)).toContain('path=a.ts')

    const searchMock = mockEnvelope({ hits: [], truncated: false })
    await searchReviewDiff({ wsId: 'ws-1', commit: 'abc1234' }, 'todo')
    expect(urlOf(searchMock)).toContain('sha=abc1234')
    expect(urlOf(searchMock)).toContain('q=todo')
  })

  it('treats an empty commit as no scope at all', async () => {
    // `''` is what a caller threading an optional prop through produces; it must
    // mean the branch, not a request for the commit named "".
    const fetchMock = mockEnvelope({ files: [] })

    await getReviewOutline({ wsId: 'ws-1', commit: '' })

    expect(urlOf(fetchMock)).not.toContain('sha=')
  })
})
