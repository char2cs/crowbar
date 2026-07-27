import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen } from '@testing-library/react'

// The API client is the seam: this component's whole job is to turn keystrokes
// into a BOUNDED number of well-formed calls to it, and to render whatever comes
// back honestly. Mocking it keeps the test about that, not about fetch.
vi.mock('@/features/git/api/review-window-api', () => ({
  searchReviewDiff: vi.fn(),
}))

import { searchReviewDiff } from '@/features/git/api/review-window-api'
import type { SearchDiffResult, SearchHit } from '@/features/git/api/review-window-api'
import {
  ReviewSearchBar,
  SEARCH_DEBOUNCE_MS,
} from '@/features/git/components/diff/review-search-bar'
import { ApiError } from '@/lib/api'

const searchMock = vi.mocked(searchReviewDiff)

function hit(path: string, lineNumber: number, preview: string, side: 'old' | 'new' = 'new') {
  return { path, side, lineNumber, preview } satisfies SearchHit
}

function resolves(hits: SearchHit[], truncated = false): void {
  searchMock.mockResolvedValue({ hits, truncated })
}

/** A search whose answer the test controls, so an EARLIER request can be made to
 *  land AFTER a later one — the race that shows stale hits. */
function deferredSearches(): {
  settle: (index: number, result: SearchDiffResult) => Promise<void>
  pending: () => number
} {
  const resolvers: Array<(r: SearchDiffResult) => void> = []
  searchMock.mockImplementation(
    () =>
      new Promise<SearchDiffResult>((resolve) => {
        resolvers.push(resolve)
      }),
  )
  return {
    pending: () => resolvers.length,
    settle: async (index, result) => {
      resolvers[index](result)
      await act(async () => {})
    },
  }
}

function renderBar(props: Partial<React.ComponentProps<typeof ReviewSearchBar>> = {}) {
  const onSelectHit = vi.fn()
  const onClose = vi.fn()
  render(<ReviewSearchBar wsId="ws-1" onSelectHit={onSelectHit} onClose={onClose} {...props} />)
  return { onSelectHit, onClose }
}

function findBox(): HTMLElement {
  return screen.getByRole('searchbox', { name: /find in diff/i })
}

function typeQuery(value: string): void {
  fireEvent.change(findBox(), { target: { value } })
}

/** Advance past the debounce and flush the resulting promise chain. */
async function runDebounce(ms: number = SEARCH_DEBOUNCE_MS): Promise<void> {
  await act(async () => {
    vi.advanceTimersByTime(ms)
  })
}

beforeEach(() => {
  vi.useFakeTimers()
  searchMock.mockReset()
  resolves([])
})

afterEach(() => {
  vi.useRealTimers()
})

describe('ReviewSearchBar — opening state', () => {
  // An empty query is what the box opens in. The daemon answers it with an empty
  // result rather than an error, and there is nothing to ask it about anyway, so
  // opening the box must cost zero requests and show zero errors.
  it('opens empty: no request, no error, no hit list', async () => {
    renderBar()

    await runDebounce()

    expect(searchMock).not.toHaveBeenCalled()
    expect(screen.queryByRole('alert')).toBeNull()
    expect(screen.queryByRole('list')).toBeNull()
    expect(findBox()).toHaveValue('')
  })

  it('clearing a query back to empty drops the hits without another request', async () => {
    resolves([hit('a.ts', 1, 'todo')])
    renderBar()

    typeQuery('todo')
    await runDebounce()
    expect(screen.getByText('a.ts')).toBeInTheDocument()

    searchMock.mockClear()
    typeQuery('')
    await runDebounce()

    expect(searchMock).not.toHaveBeenCalled()
    expect(screen.queryByText('a.ts')).toBeNull()
  })
})

describe('ReviewSearchBar — debounce', () => {
  it('debounces at 200ms', () => {
    expect(SEARCH_DEBOUNCE_MS).toBe(200)
  })

  // Each keystroke otherwise streams the whole branch diff server-side: the
  // endpoint scans 46 MB in ~700ms when nothing matches.
  it('fires ONE search for a burst of keystrokes, carrying only the final query', async () => {
    renderBar()

    typeQuery('t')
    typeQuery('to')
    typeQuery('tod')
    typeQuery('todo')
    await runDebounce()

    expect(searchMock).toHaveBeenCalledTimes(1)
    expect(searchMock).toHaveBeenCalledWith('ws-1', 'todo', expect.anything())
  })

  it('does not fire before the debounce elapses', async () => {
    renderBar()

    typeQuery('todo')
    await runDebounce(SEARCH_DEBOUNCE_MS - 1)

    expect(searchMock).not.toHaveBeenCalled()

    await runDebounce(1)

    expect(searchMock).toHaveBeenCalledTimes(1)
  })

  it('searches again for a query typed after the first settles', async () => {
    renderBar()

    typeQuery('todo')
    await runDebounce()
    typeQuery('fixme')
    await runDebounce()

    expect(searchMock).toHaveBeenCalledTimes(2)
    expect(searchMock).toHaveBeenLastCalledWith('ws-1', 'fixme', expect.anything())
  })
})

describe('ReviewSearchBar — results', () => {
  it('renders each hit with its path, line number and preview', async () => {
    resolves([hit('src/a.ts', 42, 'const todo = 1'), hit('src/b.ts', 7, '// todo later', 'old')])
    renderBar()

    typeQuery('todo')
    await runDebounce()

    expect(screen.getAllByRole('listitem')).toHaveLength(2)
    expect(screen.getByText('src/a.ts')).toBeInTheDocument()
    expect(screen.getByText('42')).toBeInTheDocument()
    expect(screen.getByText('const todo = 1')).toBeInTheDocument()
    expect(screen.getByText('src/b.ts')).toBeInTheDocument()
    expect(screen.getByText('// todo later')).toBeInTheDocument()
  })

  it('says so when nothing matched', async () => {
    resolves([])
    renderBar()

    typeQuery('zzz')
    await runDebounce()

    expect(screen.getByText(/no matches/i)).toBeInTheDocument()
  })

  // The server caps at `limit` (default 200, hard max 1000) and reports it. A
  // capped list presented as complete is a lie the reader cannot detect.
  it('reports a capped result as the FIRST n matches, never as the whole set', async () => {
    resolves(
      Array.from({ length: 200 }, (_, i) => hit(`src/f${i}.ts`, i + 1, `line ${i}`)),
      true,
    )
    renderBar()

    typeQuery('e')
    await runDebounce()

    expect(screen.getByText(/first 200 matches/i)).toBeInTheDocument()
  })

  it('claims no truncation when the server reported none', async () => {
    resolves([hit('a.ts', 1, 'x')], false)
    renderBar()

    typeQuery('x')
    await runDebounce()

    expect(screen.queryByText(/first/i)).toBeNull()
    expect(screen.getByText(/1 match/i)).toBeInTheDocument()
  })
})

describe('ReviewSearchBar — options', () => {
  it('sends no regex or case flags by default', async () => {
    renderBar()

    typeQuery('todo')
    await runDebounce()

    expect(searchMock).toHaveBeenCalledWith(
      'ws-1',
      'todo',
      expect.objectContaining({ regex: false, caseSensitive: false }),
    )
  })

  it('re-runs the search with regex enabled when the toggle is pressed', async () => {
    renderBar()

    typeQuery('a.*b')
    await runDebounce()
    fireEvent.click(screen.getByRole('button', { name: /regular expression/i }))
    await runDebounce()

    expect(searchMock).toHaveBeenLastCalledWith(
      'ws-1',
      'a.*b',
      expect.objectContaining({ regex: true }),
    )
  })

  it('re-runs the search with case sensitivity enabled when the toggle is pressed', async () => {
    renderBar()

    typeQuery('Todo')
    await runDebounce()
    fireEvent.click(screen.getByRole('button', { name: /match case/i }))
    await runDebounce()

    expect(searchMock).toHaveBeenLastCalledWith(
      'ws-1',
      'Todo',
      expect.objectContaining({ caseSensitive: true }),
    )
  })
})

describe('ReviewSearchBar — invalid regex', () => {
  // The usecase validates with regexp.Compile before spawning anything, so a
  // half-typed pattern is a cheap, ordinary 400 — not a daemon failure.
  it('surfaces a 400 as an inline message instead of throwing', async () => {
    searchMock.mockRejectedValue(new ApiError('error parsing regexp: missing closing )', 400))
    renderBar()

    typeQuery('a(b')
    await runDebounce()

    expect(screen.getByRole('alert')).toHaveTextContent(/missing closing/i)
    // The pane survives: the box is still there and still usable.
    expect(findBox()).toBeInTheDocument()
  })

  it('clears the error once a later query succeeds', async () => {
    searchMock.mockRejectedValueOnce(new ApiError('error parsing regexp: missing closing )', 400))
    renderBar()

    typeQuery('a(b')
    await runDebounce()
    expect(screen.getByRole('alert')).toBeInTheDocument()

    resolves([hit('a.ts', 3, 'ab')])
    typeQuery('a(b)')
    await runDebounce()

    expect(screen.queryByRole('alert')).toBeNull()
    expect(screen.getByText('a.ts')).toBeInTheDocument()
  })
})

describe('ReviewSearchBar — navigation', () => {
  const HITS = [hit('a.ts', 1, 'one'), hit('b.ts', 2, 'two'), hit('c.ts', 3, 'three')]

  async function withHits() {
    resolves(HITS)
    const bar = renderBar()
    typeQuery('x')
    await runDebounce()
    return bar
  }

  it('does not jump anywhere merely because results arrived', async () => {
    const { onSelectHit } = await withHits()

    expect(onSelectHit).not.toHaveBeenCalled()
  })

  it('next walks the hit list and wraps', async () => {
    const { onSelectHit } = await withHits()
    const next = screen.getByRole('button', { name: /next match/i })

    fireEvent.click(next)
    fireEvent.click(next)
    fireEvent.click(next)
    fireEvent.click(next)

    expect(onSelectHit.mock.calls.map((c) => c[0])).toEqual([HITS[0], HITS[1], HITS[2], HITS[0]])
  })

  it('previous walks backwards and wraps to the last hit', async () => {
    const { onSelectHit } = await withHits()

    fireEvent.click(screen.getByRole('button', { name: /previous match/i }))

    expect(onSelectHit).toHaveBeenCalledWith(HITS[2])
  })

  it('clicking a hit selects it', async () => {
    const { onSelectHit } = await withHits()

    fireEvent.click(screen.getByText('b.ts'))

    expect(onSelectHit).toHaveBeenCalledWith(HITS[1])
  })

  it('marks the selected hit as current', async () => {
    await withHits()

    fireEvent.click(screen.getByRole('button', { name: /next match/i }))

    expect(screen.getByRole('button', { current: true })).toHaveTextContent('a.ts')
  })

  it('shows the position within the hit list', async () => {
    await withHits()

    fireEvent.click(screen.getByRole('button', { name: /next match/i }))
    fireEvent.click(screen.getByRole('button', { name: /next match/i }))

    expect(screen.getByText('2/3')).toBeInTheDocument()
  })

  it('disables next and previous when there is nothing to walk', async () => {
    resolves([])
    renderBar()

    typeQuery('zzz')
    await runDebounce()

    expect(screen.getByRole('button', { name: /next match/i })).toBeDisabled()
    expect(screen.getByRole('button', { name: /previous match/i })).toBeDisabled()
  })

  it('Enter advances, Shift+Enter goes back, Escape closes', async () => {
    const { onSelectHit, onClose } = await withHits()

    fireEvent.keyDown(findBox(), { key: 'Enter' })
    expect(onSelectHit).toHaveBeenLastCalledWith(HITS[0])

    fireEvent.keyDown(findBox(), { key: 'Enter', shiftKey: true })
    expect(onSelectHit).toHaveBeenLastCalledWith(HITS[2])

    fireEvent.keyDown(findBox(), { key: 'Escape' })
    expect(onClose).toHaveBeenCalledTimes(1)
  })
})

describe('ReviewSearchBar — superseded requests', () => {
  // A slow early query landing after a fast later one is the classic find-box
  // race: the reader sees hits for a query they no longer have typed.
  it('ignores a stale response that lands after a newer one', async () => {
    const searches = deferredSearches()
    renderBar()

    typeQuery('slow')
    await runDebounce()
    typeQuery('fast')
    await runDebounce()
    expect(searches.pending()).toBe(2)

    // The LATER request answers first.
    await searches.settle(1, { hits: [hit('fast.ts', 2, 'fast hit')], truncated: false })
    expect(screen.getByText('fast.ts')).toBeInTheDocument()

    // Then the earlier one finally lands — and must change nothing.
    await searches.settle(0, { hits: [hit('slow.ts', 1, 'slow hit')], truncated: false })

    expect(screen.queryByText('slow.ts')).toBeNull()
    expect(screen.getByText('fast.ts')).toBeInTheDocument()
  })

  it('ignores a stale FAILURE that lands after a newer success', async () => {
    const resolvers: Array<{ resolve: (r: SearchDiffResult) => void; reject: (e: Error) => void }> =
      []
    searchMock.mockImplementation(
      () => new Promise<SearchDiffResult>((resolve, reject) => resolvers.push({ resolve, reject })),
    )
    renderBar()

    typeQuery('a(b')
    await runDebounce()
    typeQuery('a(b)')
    await runDebounce()

    resolvers[1].resolve({ hits: [hit('ok.ts', 1, 'ok')], truncated: false })
    await act(async () => {})
    resolvers[0].reject(new ApiError('error parsing regexp: missing closing )', 400))
    await act(async () => {})

    expect(screen.queryByRole('alert')).toBeNull()
    expect(screen.getByText('ok.ts')).toBeInTheDocument()
  })

  it('clearing the box supersedes an in-flight search', async () => {
    const searches = deferredSearches()
    renderBar()

    typeQuery('todo')
    await runDebounce()
    typeQuery('')
    await runDebounce()

    await searches.settle(0, { hits: [hit('late.ts', 9, 'late')], truncated: false })

    expect(screen.queryByText('late.ts')).toBeNull()
  })
})
