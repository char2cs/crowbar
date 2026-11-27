import type { ReactNode } from 'react'
import { act, fireEvent, render, renderHook, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  toAnnotationSide,
  useReviewAnnotations,
} from '@/features/git/components/diff/use-review-annotations'
import { ReviewCodeView } from '@/features/git/components/diff/review-code-view'
import type { FileOutline } from '@/features/git/api/review-window-api'
import type { GitDiff } from '@/features/git/types/git-types'
import type { ReviewThread } from '@/features/workspace/stores/slices/branch-review-slice'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { clearReviewThreads, putReviewThread } from '@/__tests__/__fixtures__/review-threads'
import { warmDiffHighlighter } from '@/__tests__/__fixtures__/diff-highlighter'

await warmDiffHighlighter()

const mocks = vi.hoisted(() => ({
  openThread: vi.fn(),
  replyToThread: vi.fn(),
  setThreadResolved: vi.fn(),
  deleteThread: vi.fn(),
  deleteMessage: vi.fn(),
  editMessage: vi.fn(),
  getIdentity: vi.fn(),
  getReviewPatch: vi.fn(),
}))

vi.mock('@/features/git/api/review-api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/features/git/api/review-api')>()
  return {
    ...actual,
    openThread: mocks.openThread,
    replyToThread: mocks.replyToThread,
    setThreadResolved: mocks.setThreadResolved,
    deleteThread: mocks.deleteThread,
    deleteMessage: mocks.deleteMessage,
    editMessage: mocks.editMessage,
  }
})

vi.mock('@/features/git/api/identity-api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/features/git/api/identity-api')>()
  return { ...actual, getIdentity: mocks.getIdentity }
})

vi.mock('@/features/git/api/review-window-api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/features/git/api/review-window-api')>()
  return { ...actual, getReviewPatch: mocks.getReviewPatch }
})

const VIEWPORT_WIDTH = 900
const VIEWPORT_HEIGHT = 800
const originalGetBoundingClientRect = HTMLElement.prototype.getBoundingClientRect
const originalScrollTo = Element.prototype.scrollTo

beforeEach(() => {
  Element.prototype.scrollTo = function scrollTo(
    this: Element,
    optionsOrX?: ScrollToOptions | number,
    y?: number,
  ) {
    const top = typeof optionsOrX === 'object' ? optionsOrX.top : y
    if (typeof top === 'number') this.scrollTop = top
  } as Element['scrollTo']

  const rect = {
    top: 0,
    left: 0,
    right: VIEWPORT_WIDTH,
    bottom: VIEWPORT_HEIGHT,
    width: VIEWPORT_WIDTH,
    height: VIEWPORT_HEIGHT,
    x: 0,
    y: 0,
  }
  HTMLElement.prototype.getBoundingClientRect = function getBoundingClientRect() {
    return { ...rect, toJSON: () => rect } as DOMRect
  }

  for (const mock of Object.values(mocks)) mock.mockReset()
  mocks.getIdentity.mockResolvedValue({
    login: 'alice',
    displayName: 'Alice',
    avatarUrl: 'https://example.invalid/alice.png',
  })
  mocks.openThread.mockResolvedValue(undefined)
  mocks.setThreadResolved.mockResolvedValue(undefined)
  mocks.getReviewPatch.mockImplementation(async (_wsId: string, path: string) => ({
    patch: patchFor(path),
    truncated: false,
  }))
})

afterEach(() => {
  HTMLElement.prototype.getBoundingClientRect = originalGetBoundingClientRect
  Element.prototype.scrollTo = originalScrollTo
})

function makeThread(overrides: Partial<ReviewThread> = {}): ReviewThread {
  return {
    id: 'thread-1',
    filePath: 'src/pkg/file0.ts',
    lineNumber: 2,
    startLine: 2,
    endLine: 2,
    side: 'new',
    messages: [
      {
        id: 'msg-1',
        author: 'alice',
        isAgent: false,
        body: 'crowbarThreadBody',
        createdAt: '2026-07-27T00:00:00Z',
      },
    ],
    isResolved: false,
    ...overrides,
  }
}

const pathOf = (i: number) => `src/pkg/file${i}.ts`
const markerOf = (i: number) => `crowbarMarker${i}`

function patchFor(path: string): string {
  if (path === DEEP_FILE) return deepPatch()
  const i = Number(/file(\d+)\.ts$/.exec(path)?.[1] ?? 0)
  return `diff --git a/${path} b/${path}
index 1111111..2222222 100644
--- a/${path}
+++ b/${path}
@@ -1,3 +1,3 @@
 export function ${markerOf(i)}() {
-  return "before"
+  return "after"
 }
`
}

function textFile(i: number): GitDiff {
  return {
    file_path: pathOf(i),
    is_new: false,
    is_deleted: false,
    is_renamed: false,
    lines: [],
    additions: 1,
    deletions: 1,
  }
}

function textOutline(i: number): FileOutline {
  return {
    path: pathOf(i),
    hunks: [{ oldStart: 1, oldLines: 3, newStart: 1, newLines: 3 }],
    isPartial: false,
    isBinary: false,
  }
}

const DEEP_FILE = 'src/pkg/deep.ts'
const DEEP_TAIL_LINES = 500
const DEEP_THREAD_LINE = 1_500
const DEEP_MARKER = 'crowbarDeepMarker'

function deepPatch(): string {
  const context = DEEP_THREAD_LINE - 1
  const lines = [
    `diff --git a/${DEEP_FILE} b/${DEEP_FILE}`,
    'index 1111111..2222222 100644',
    `--- a/${DEEP_FILE}`,
    `+++ b/${DEEP_FILE}`,
    `@@ -1,${context + DEEP_TAIL_LINES} +1,${context + 1 + DEEP_TAIL_LINES} @@`,
  ]
  for (let n = 1; n <= context; n++) lines.push(` const x${n} = ${n}`)
  lines.push(`+const added = "${DEEP_MARKER}"`)
  for (let n = 1; n <= DEEP_TAIL_LINES; n++) lines.push(` const y${n} = ${n}`)
  return `${lines.join('\n')}\n`
}

function deepFile(): GitDiff {
  return {
    file_path: DEEP_FILE,
    is_new: false,
    is_deleted: false,
    is_renamed: false,
    lines: [],
    additions: 1,
    deletions: 0,
  }
}

function deepOutline(): FileOutline {
  const context = DEEP_THREAD_LINE - 1
  return {
    path: DEEP_FILE,
    hunks: [
      {
        oldStart: 1,
        oldLines: context + DEEP_TAIL_LINES,
        newStart: 1,
        newLines: context + 1 + DEEP_TAIL_LINES,
      },
    ],
    isPartial: false,
    isBinary: false,
  }
}

function seedStore(threads: readonly ReviewThread[]): WorkspaceStore {
  clearReviewThreads()
  for (const thread of threads) putReviewThread('ws-1', thread)
  return createWorkspaceStore('ws-1')
}

function renderLayer(threads: readonly ReviewThread[] = []) {
  const store = seedStore(threads)
  const wrapper = ({ children }: { children: ReactNode }) => (
    <WorkspaceStoreContext.Provider value={store}>{children}</WorkspaceStoreContext.Provider>
  )
  return {
    store,
    ...renderHook(() => useReviewAnnotations({ wsId: 'ws-1' }), { wrapper }),
  }
}

function renderSurface(
  files: readonly GitDiff[],
  outline: readonly FileOutline[],
  threads: readonly ReviewThread[] = [],
) {
  const store = seedStore(threads)
  const view = render(
    <WorkspaceStoreContext.Provider value={store}>
      <ReviewCodeView wsId="ws-1" files={files} outline={outline} />
    </WorkspaceStoreContext.Provider>,
  )
  return { store, ...view }
}

function shadowText(container: HTMLElement): string {
  return [...container.querySelectorAll('diffs-container')]
    .map((host) => (host as HTMLElement).shadowRoot?.textContent ?? '')
    .join('\n')
}

const DEEP_RENDER = { timeout: 20_000 }

async function waitForCode(container: HTMLElement, marker: string, timeout = 5_000): Promise<void> {
  await waitFor(
    () => {
      expect(shadowText(container), `code rows for ${marker} have not rendered`).toContain(marker)
    },
    { timeout },
  )
}

function projectedAnnotationSlots(container: HTMLElement): string[] {
  return [...container.querySelectorAll('diffs-container')].flatMap((host) =>
    [
      ...((host as HTMLElement).shadowRoot?.querySelectorAll('slot[name^="annotation-"]') ?? []),
    ].map((slot) => slot.getAttribute('name') ?? ''),
  )
}

function fetchesOf(path: string): number {
  return mocks.getReviewPatch.mock.calls.filter((call) => call[1] === path).length
}

describe('review thread ↔ annotation side map', () => {
  it("maps a thread's old side to deletions and its new side to additions", () => {
    expect(toAnnotationSide('old')).toBe('deletions')
    expect(toAnnotationSide('new')).toBe('additions')
  })
})

describe('threads as annotations', () => {
  it('carries the line number and the thread itself as metadata, and hands the thread back', () => {
    const thread = makeThread({ filePath: 'src/a.ts', lineNumber: 42 })
    const { result } = renderLayer([thread])

    const annotation = result.current.annotationsByPath.get('src/a.ts')?.[0]
    expect(annotation?.lineNumber).toBe(42)
    expect(annotation?.side).toBe('additions')
    expect(annotation?.metadata).toEqual(thread)
    expect(result.current.threadsFor('src/a.ts')).toEqual([thread])
  })

  it('anchors old-side threads to deletions and returns each thread unchanged', () => {
    const threads = (['old', 'new'] as const).map((side) =>
      makeThread({ id: `thread-${side}`, filePath: `src/${side}.ts`, side, lineNumber: 7 }),
    )
    const { result } = renderLayer(threads)

    expect(result.current.annotationsByPath.get('src/old.ts')?.[0].side).toBe('deletions')
    expect(result.current.annotationsByPath.get('src/new.ts')?.[0].side).toBe('additions')
    for (const thread of threads) {
      expect(result.current.threadsFor(thread.filePath)).toEqual([thread])
    }
  })
})

describe('grouping threads by file', () => {
  it('groups annotations under each thread file path, ordered by line', () => {
    const a = makeThread({ id: 'a', filePath: 'src/a.ts', lineNumber: 9 })
    const b = makeThread({ id: 'b', filePath: 'src/a.ts', lineNumber: 3 })
    const c = makeThread({ id: 'c', filePath: 'src/b.ts', lineNumber: 1, side: 'old' })

    const grouped = renderLayer([a, b, c]).result.current.annotationsByPath

    expect([...grouped.keys()].sort()).toEqual(['src/a.ts', 'src/b.ts'])
    expect(grouped.get('src/a.ts')?.map((x) => x.lineNumber)).toEqual([3, 9])
    expect(grouped.get('src/b.ts')?.[0].side).toBe('deletions')
  })

  it('counts threads per file, including files the diff never materialises', () => {
    const { threadCounts } = renderLayer([
      makeThread({ id: 'a', filePath: 'src/a.ts' }),
      makeThread({ id: 'b', filePath: 'src/a.ts' }),
      makeThread({ id: 'c', filePath: 'never/materialised.ts' }),
    ]).result.current

    expect(threadCounts).toEqual({ 'src/a.ts': 2, 'never/materialised.ts': 1 })
  })

  it('ignores un-anchored (Conversation-only) threads in annotations and counts', () => {
    const general = makeThread({ id: 'g', filePath: '', lineNumber: 0 })
    const anchored = makeThread({ id: 'a', filePath: 'src/a.ts', lineNumber: 2 })

    const layer = renderLayer([general, anchored]).result.current

    expect([...layer.annotationsByPath.keys()]).toEqual(['src/a.ts'])
    expect(layer.threadCounts).toEqual({ 'src/a.ts': 1 })
  })
})

describe('useReviewAnnotations', () => {
  it('exposes stored threads as per-file annotations', () => {
    const { result } = renderLayer([makeThread({ filePath: 'src/a.ts', lineNumber: 5 })])

    const annotations = result.current.annotationsByPath.get('src/a.ts')
    expect(annotations).toHaveLength(1)
    expect(annotations?.[0]).toMatchObject({ side: 'additions', lineNumber: 5 })
    expect(result.current.threadCounts['src/a.ts']).toBe(1)
  })

  it('opens a draft from a line selection and posts it on the NEW side', async () => {
    const { result } = renderLayer()

    act(() => {
      result.current.onSelectedLinesChange({
        id: 'src/a.ts',
        range: { start: 3, end: 7, side: 'additions' },
      })
    })

    const draft = result.current.annotationsByPath.get('src/a.ts')?.[0]
    expect(draft).toMatchObject({ side: 'additions', lineNumber: 7 })
    expect(result.current.draft).not.toBeNull()
    expect(draft?.metadata).toBe(result.current.draft)

    await act(async () => {
      await result.current.submitDraft('looks wrong')
    })

    expect(mocks.openThread).toHaveBeenCalledWith('ws-1', {
      filePath: 'src/a.ts',
      line: 7,
      startLine: 3,
      endLine: 7,
      side: 'new',
      author: 'alice',
      isAgent: false,
      body: 'looks wrong',
    })
  })

  it('posts a deletions-side selection against the OLD side', async () => {
    const { result } = renderLayer()

    act(() => {
      result.current.onSelectedLinesChange({
        id: 'src/a.ts',
        range: { start: 4, end: 4, side: 'deletions' },
      })
    })
    await act(async () => {
      await result.current.submitDraft('why was this removed')
    })

    expect(mocks.openThread).toHaveBeenCalledWith(
      'ws-1',
      expect.objectContaining({ side: 'old', line: 4, startLine: 4, endLine: 4 }),
    )
  })

  it('normalises a backwards selection so start never exceeds end', async () => {
    const { result } = renderLayer()

    act(() => {
      result.current.onSelectedLinesChange({
        id: 'src/a.ts',
        range: { start: 9, end: 2, side: 'additions' },
      })
    })
    await act(async () => {
      await result.current.submitDraft('range')
    })

    expect(mocks.openThread).toHaveBeenCalledWith(
      'ws-1',
      expect.objectContaining({ startLine: 2, endLine: 9, line: 9 }),
    )
  })

  it('clears the draft when the selection is cleared', () => {
    const { result } = renderLayer()

    act(() => {
      result.current.onSelectedLinesChange({
        id: 'src/a.ts',
        range: { start: 1, end: 1, side: 'additions' },
      })
    })
    expect(result.current.draft).not.toBeNull()

    act(() => {
      result.current.onSelectedLinesChange(null)
    })
    expect(result.current.draft).toBeNull()
    expect(result.current.annotationsByPath.get('src/a.ts')).toBeUndefined()
  })

  it('opens a draft from the hovered-line "+" gutter utility', () => {
    const { result } = renderLayer()

    const node = result.current.renderGutterUtility(() => ({ lineNumber: 6, side: 'deletions' }), {
      id: 'src/a.ts',
      type: 'diff',
      fileDiff: { name: 'src/a.ts' } as never,
    })
    render(<>{node}</>)
    fireEvent.click(screen.getByRole('button', { name: /comment/i }))

    expect(result.current.draft).toMatchObject({
      filePath: 'src/a.ts',
      side: 'old',
      lineNumber: 6,
    })
  })

  it('renders an existing thread through ReviewThreadItem, wired to the review API', async () => {
    const thread = makeThread({ id: 'thread-9' })
    const { result } = renderLayer([thread])

    const annotation = result.current.annotationsByPath.get(thread.filePath)?.[0]
    if (!annotation) throw new Error('thread has no annotation')
    render(<>{result.current.renderAnnotation(annotation)}</>)

    expect(screen.getByText('crowbarThreadBody')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /resolve/i }))

    await waitFor(() => {
      expect(mocks.setThreadResolved).toHaveBeenCalledWith('ws-1', 'thread-9', true)
    })
  })

  it('renders the draft as a composer rather than a thread', () => {
    const { result } = renderLayer()

    act(() => {
      result.current.onSelectedLinesChange({
        id: 'src/a.ts',
        range: { start: 2, end: 2, side: 'additions' },
      })
    })
    const draftAnnotation = result.current.annotationsByPath.get('src/a.ts')![0]
    render(<>{result.current.renderAnnotation(draftAnnotation)}</>)

    expect(screen.getByRole('button', { name: 'Comment' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /resolve/i })).toBeNull()
  })

  it('surfaces a failed post instead of silently dropping the draft', async () => {
    mocks.openThread.mockRejectedValue(new Error('boom'))
    const { result } = renderLayer()

    act(() => {
      result.current.onSelectedLinesChange({
        id: 'src/a.ts',
        range: { start: 1, end: 1, side: 'additions' },
      })
    })
    await act(async () => {
      await result.current.submitDraft('nope')
    })

    expect(result.current.error?.msg).toMatch(/failed/i)
    expect(result.current.draft).not.toBeNull()
  })
})

describe('ReviewCodeView thread annotations', () => {
  it('anchors a new-side thread to the additions half of its file', async () => {
    const { container } = renderSurface(
      [textFile(0)],
      [textOutline(0)],
      [makeThread({ filePath: pathOf(0), lineNumber: 2, side: 'new' })],
    )

    await waitForCode(container, markerOf(0))

    expect(screen.getByText('crowbarThreadBody')).toBeInTheDocument()
    expect(projectedAnnotationSlots(container)).toContain('annotation-additions-2')
  })

  it('anchors an old-side thread to the deletions half of its file', async () => {
    const { container } = renderSurface(
      [textFile(0)],
      [textOutline(0)],
      [makeThread({ filePath: pathOf(0), lineNumber: 2, side: 'old' })],
    )

    await waitForCode(container, markerOf(0))

    expect(projectedAnnotationSlots(container)).toContain('annotation-deletions-2')
  })

  it('picks up a thread that arrives after mount, as the stream delivers it', async () => {
    const { container } = renderSurface([textFile(0)], [textOutline(0)])

    await waitForCode(container, markerOf(0))
    expect(projectedAnnotationSlots(container)).toHaveLength(0)

    act(() => {
      putReviewThread('ws-1', makeThread({ filePath: pathOf(0), lineNumber: 2 }))
    })

    await waitFor(() => {
      expect(projectedAnnotationSlots(container)).toContain('annotation-additions-2')
    })
    expect(screen.getByText('crowbarThreadBody')).toBeInTheDocument()
  })

  it('advertises a thread count on the header of a file whose patch never loaded', async () => {
    mocks.getReviewPatch.mockImplementation(async (_wsId: string, path: string) => {
      if (path === pathOf(1)) throw new Error('boom')
      return { patch: patchFor(path), truncated: false }
    })

    const { container } = renderSurface(
      [textFile(0), textFile(1)],
      [textOutline(0), textOutline(1)],
      [
        makeThread({ id: 't1', filePath: pathOf(1), lineNumber: 2 }),
        makeThread({ id: 't2', filePath: pathOf(1), lineNumber: 3 }),
      ],
    )

    await waitForCode(container, markerOf(0))

    expect(await screen.findByRole('button', { name: '2 comments' })).toBeInTheDocument()
    expect(projectedAnnotationSlots(container)).toHaveLength(0)
  })

  it("materialises a thread's file BEFORE scrolling to its line", DEEP_RENDER, async () => {
    mocks.getReviewPatch.mockImplementation(async (_wsId: string, path: string) => {
      if (path === DEEP_FILE && fetchesOf(DEEP_FILE) === 1) throw new Error('boom')
      return { patch: patchFor(path), truncated: false }
    })

    const { container } = renderSurface(
      [textFile(0), deepFile()],
      [textOutline(0), deepOutline()],
      [
        makeThread({
          id: 'deep',
          filePath: DEEP_FILE,
          lineNumber: DEEP_THREAD_LINE,
          side: 'new',
        }),
      ],
    )

    await waitForCode(container, markerOf(0))
    expect(shadowText(container)).not.toContain(DEEP_MARKER)

    fireEvent.click(await screen.findByRole('button', { name: '1 comment' }))

    await waitForCode(container, DEEP_MARKER, DEEP_RENDER.timeout)
    expect(fetchesOf(DEEP_FILE)).toBeGreaterThan(1)
  })
})
