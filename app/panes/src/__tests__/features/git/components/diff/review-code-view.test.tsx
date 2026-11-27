import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  ReviewCodeView,
  REVIEW_TOKENIZE_MAX_LENGTH,
  REVIEW_TOKENIZE_MAX_LINE_LENGTH,
} from '@/features/git/components/diff/review-code-view'
import {
  buildPlaceholderFileDiff,
  partitionReviewFiles,
} from '@/features/git/lib/review-placeholder'
import type { FileOutline } from '@/features/git/api/review-window-api'
import { PATCH_LINE_CAP } from '@/features/git/lib/patch-window'
import type { GitDiff } from '@/features/git/types/git-types'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'

const ROW_HEIGHT_UPPER_BOUND = 64

const mocks = vi.hoisted(() => ({
  getReviewPatch: vi.fn(),
}))

vi.mock('@/features/git/api/review-window-api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/features/git/api/review-window-api')>()
  return { ...actual, getReviewPatch: mocks.getReviewPatch }
})

const VIEWPORT_WIDTH = 900
const VIEWPORT_HEIGHT = 800
const originalGetBoundingClientRect = HTMLElement.prototype.getBoundingClientRect

beforeEach(() => {
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
  mocks.getReviewPatch.mockReset()
  mocks.getReviewPatch.mockImplementation(async (_wsId: string, path: string) => ({
    patch: patchFor(path),
    truncated: false,
  }))
})

afterEach(() => {
  HTMLElement.prototype.getBoundingClientRect = originalGetBoundingClientRect
})

const pathOf = (i: number) => `src/pkg/file${i}.ts`

const markerOf = (i: number) => `crowbarMarker${i}`

function patchFor(path: string): string {
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

function manyFiles(count: number): { files: GitDiff[]; outline: FileOutline[] } {
  return {
    files: Array.from({ length: count }, (_, i) => textFile(i)),
    outline: Array.from({ length: count }, (_, i) => textOutline(i)),
  }
}

function renderSurface(files: GitDiff[], outline: FileOutline[]) {
  const store = createWorkspaceStore('ws-1')
  const view = render(
    <WorkspaceStoreContext.Provider value={store}>
      <ReviewCodeView wsId="ws-1" files={files} outline={outline} />
    </WorkspaceStoreContext.Provider>,
  )
  return {
    ...view,
    update(nextFiles: GitDiff[], nextOutline: FileOutline[]) {
      view.rerender(
        <WorkspaceStoreContext.Provider value={store}>
          <ReviewCodeView wsId="ws-1" files={nextFiles} outline={nextOutline} />
        </WorkspaceStoreContext.Provider>,
      )
    },
  }
}

function shadowText(container: HTMLElement): string {
  return [...container.querySelectorAll('diffs-container')]
    .map((host) => (host as HTMLElement).shadowRoot?.textContent ?? '')
    .join('\n')
}

async function waitForCode(container: HTMLElement, marker: string): Promise<void> {
  await waitFor(() => {
    expect(shadowText(container), `code rows for ${marker} have not rendered`).toContain(marker)
  })
}

function fetchedPaths(): string[] {
  return mocks.getReviewPatch.mock.calls.map((call) => call[1] as string)
}

function scrollerOf(container: HTMLElement): HTMLElement {
  const root = container.querySelector('[data-review-code-view-scroller]') as HTMLElement | null
  expect(root, 'expected the CodeView scroll container').toBeTruthy()
  return root!
}

function reservedHeight(container: HTMLElement): number {
  const scaffold = scrollerOf(container).firstElementChild as HTMLElement | null
  return Number.parseFloat(scaffold?.style.height || '0')
}

describe('partitionReviewFiles', () => {
  it('produces exactly one entry per file, in the summary order', () => {
    const { files, outline } = manyFiles(5)
    const entries = partitionReviewFiles(files, outline)

    expect(entries).toHaveLength(files.length)
    expect(entries.map((e) => e.path)).toEqual(files.map((f) => f.file_path))
    expect(entries.every((e) => e.kind === 'diff')).toBe(true)
  })

  it('routes binaries away from the diff renderer, images separately from other binaries', () => {
    const files: GitDiff[] = [
      { ...textFile(0), file_path: 'assets/logo.png' },
      { ...textFile(1), file_path: 'assets/blob.bin' },
      textFile(2),
    ]
    const outline: FileOutline[] = [
      { path: 'assets/logo.png', hunks: [], isPartial: false, isBinary: true },
      { path: 'assets/blob.bin', hunks: [], isPartial: false, isBinary: true },
      textOutline(2),
    ]

    expect(partitionReviewFiles(files, outline).map((e) => e.kind)).toEqual([
      'image',
      'binary',
      'diff',
    ])
  })
})

describe('buildPlaceholderFileDiff', () => {
  it('reserves geometry from hunk shapes alone, holding no line content', () => {
    const fileDiff = buildPlaceholderFileDiff(textFile(0), textOutline(0))

    expect(fileDiff.additionLines).toEqual([])
    expect(fileDiff.deletionLines).toEqual([])
    expect(fileDiff.unifiedLineCount).toBe(4)
    expect(fileDiff.splitLineCount).toBe(3)
  })

  it('tops a PARTIAL file up from the summary counts, since its outline is a lower bound', () => {
    const file: GitDiff = { ...textFile(0), additions: 40_000, deletions: 20_000 }
    const capped: FileOutline = { ...textOutline(0), isPartial: true }

    const partial = buildPlaceholderFileDiff(file, capped)
    const complete = buildPlaceholderFileDiff(file, { ...capped, isPartial: false })

    expect(partial.unifiedLineCount).toBeGreaterThan(complete.unifiedLineCount)
    expect(partial.unifiedLineCount).toBeLessThanOrEqual(PATCH_LINE_CAP)
  })

  it("reserves the capped height but still reports the file's TRUE ± counts", () => {
    const monster: GitDiff = { ...textFile(0), additions: 420_000, deletions: 0 }
    const oneHugeHunk: FileOutline = {
      ...textOutline(0),
      hunks: [{ oldStart: 0, oldLines: 0, newStart: 1, newLines: 420_000 }],
    }

    const placeholder = buildPlaceholderFileDiff(monster, oneHugeHunk)

    expect(placeholder.unifiedLineCount).toBeLessThanOrEqual(PATCH_LINE_CAP)
    const reported = placeholder.hunks.reduce((n, h) => n + h.additionLines, 0)
    expect(reported).toBe(420_000)
  })
})

describe('tokenisation caps', () => {
  it('caps line length far below a minified 657,780-character line', () => {
    expect(REVIEW_TOKENIZE_MAX_LINE_LENGTH).toBeGreaterThan(0)
    expect(REVIEW_TOKENIZE_MAX_LINE_LENGTH).toBeLessThan(657_780)
  })

  it('caps total diff length so a generated file degrades to plain text', () => {
    expect(REVIEW_TOKENIZE_MAX_LENGTH).toBeGreaterThan(0)
    expect(REVIEW_TOKENIZE_MAX_LENGTH).toBeLessThan(420_000)
  })
})

describe('ReviewCodeView windowing', () => {
  it('renders the head of the diff and fetches only the files near it', async () => {
    const { files, outline } = manyFiles(60)
    const { container } = renderSurface(files, outline)

    await waitForCode(container, markerOf(0))

    const fetched = fetchedPaths()
    expect(fetched).toContain(pathOf(0))
    expect(fetched).not.toContain(pathOf(59))
    expect(new Set(fetched).size).toBeLessThan(files.length)
  })

  it('materialises the files in view on first paint, and only those', async () => {
    const { files, outline } = manyFiles(200)
    const { container } = renderSurface(files, outline)

    await waitForCode(container, markerOf(0))

    const fetched = fetchedPaths()
    expect(fetched).toContain(pathOf(0))
    expect(fetched.length).toBeLessThan(60)
  })

  it('re-reserves space when a refreshed summary resizes a file still on placeholder', async () => {
    const { files, outline } = manyFiles(3)
    mocks.getReviewPatch.mockImplementation(async (_wsId: string, path: string) => {
      if (path === pathOf(0)) return new Promise<never>(() => {})
      return { patch: patchFor(path), truncated: false }
    })

    const { container, update } = renderSurface(files, outline)
    await waitForCode(container, markerOf(1))
    const before = reservedHeight(container)
    expect(before).toBeGreaterThan(0)

    update(
      files.map((f, i) => (i === 0 ? { ...f, additions: 400, deletions: 0 } : f)),
      outline.map((o, i) =>
        i === 0 ? { ...o, hunks: [{ oldStart: 1, oldLines: 0, newStart: 1, newLines: 400 }] } : o,
      ),
    )

    await waitFor(() => {
      expect(reservedHeight(container) - before).toBeGreaterThan(396 * 20 * 0.9)
    })
  })

  it('reserves MORE scroll space for a partial file than its outline alone describes', async () => {
    const file: GitDiff = { ...textFile(0), additions: 40_000, deletions: 20_000 }
    mocks.getReviewPatch.mockImplementation(async () => new Promise<never>(() => {}))

    const complete = renderSurface([file], [textOutline(0)])
    const partial = renderSurface([file], [{ ...textOutline(0), isPartial: true }])

    await waitFor(() => {
      expect(reservedHeight(complete.container)).toBeGreaterThan(0)
      expect(reservedHeight(partial.container)).toBeGreaterThan(0)
    })
    expect(reservedHeight(partial.container)).toBeGreaterThan(reservedHeight(complete.container))
    expect(reservedHeight(partial.container)).toBeLessThanOrEqual(
      PATCH_LINE_CAP * ROW_HEIGHT_UPPER_BOUND,
    )
  })
})

describe('ReviewCodeView truncated patches', () => {
  const MONSTER = 'dist/bundle.js'

  const HEADER_ONLY = `diff --git a/${MONSTER} b/${MONSTER}
index 1111111..2222222 100644
--- a/${MONSTER}
+++ b/${MONSTER}
`

  const FULL = `diff --git a/${MONSTER} b/${MONSTER}
index 1111111..2222222 100644
--- a/${MONSTER}
+++ b/${MONSTER}
@@ -1,2 +1,2 @@
 const bundled = 1
-const marker = "before"
+const marker = "crowbarMonsterMarker"
`

  function monster(): { files: GitDiff[]; outline: FileOutline[] } {
    return {
      files: [
        {
          file_path: MONSTER,
          is_new: false,
          is_deleted: false,
          is_renamed: false,
          lines: [],
          additions: 210_000,
          deletions: 210_000,
        },
      ],
      outline: [
        {
          path: MONSTER,
          hunks: [{ oldStart: 1, oldLines: 210_000, newStart: 1, newLines: 210_000 }],
          isPartial: false,
          isBinary: false,
        },
      ],
    }
  }

  it('offers "show all" for a header-only capped patch instead of an empty file', async () => {
    mocks.getReviewPatch.mockImplementation(async (_wsId: string, _path: string, max?: number) =>
      max === 0 ? { patch: FULL, truncated: false } : { patch: HEADER_ONLY, truncated: true },
    )

    const { files, outline } = monster()
    const { container } = renderSurface(files, outline)

    const showAll = await screen.findByRole('button', { name: /show all/i })
    expect(shadowText(container)).not.toContain('crowbarMonsterMarker')

    fireEvent.click(showAll)

    await waitForCode(container, 'crowbarMonsterMarker')
    expect(mocks.getReviewPatch).toHaveBeenCalledWith(
      { wsId: 'ws-1', commit: undefined },
      MONSTER,
      0,
    )
  })
})

describe('ReviewCodeView failed patches', () => {
  it('surfaces a failed fetch as a retry that asks for the CAPPED patch again', async () => {
    let attempt = 0
    mocks.getReviewPatch.mockImplementation(async (_wsId: string, path: string) => {
      if (path !== pathOf(0)) return { patch: patchFor(path), truncated: false }
      attempt++
      if (attempt === 1) throw new Error('boom')
      return { patch: patchFor(path), truncated: false }
    })

    const { files, outline } = manyFiles(2)
    const { container } = renderSurface(files, outline)

    const retry = await screen.findByRole('button', { name: /retry/i })
    fireEvent.click(retry)

    await waitForCode(container, markerOf(0))
    expect(mocks.getReviewPatch).not.toHaveBeenCalledWith(
      { wsId: 'ws-1', commit: undefined },
      pathOf(0),
      0,
    )
  })
})

describe('ReviewCodeView binary files', () => {
  it('never feeds a binary to the diff renderer and renders a plain row instead', async () => {
    const files: GitDiff[] = [
      { ...textFile(0), file_path: 'assets/blob.bin' },
      { ...textFile(1), file_path: 'assets/logo.png' },
      textFile(2),
    ]
    const outline: FileOutline[] = [
      { path: 'assets/blob.bin', hunks: [], isPartial: false, isBinary: true },
      { path: 'assets/logo.png', hunks: [], isPartial: false, isBinary: true },
      textOutline(2),
    ]

    const { container } = renderSurface(files, outline)

    await waitForCode(container, markerOf(2))

    expect(screen.getByText('assets/blob.bin')).toBeInTheDocument()
    expect(screen.getByText('assets/logo.png')).toBeInTheDocument()
    expect(fetchedPaths()).not.toContain('assets/blob.bin')
    expect(fetchedPaths()).not.toContain('assets/logo.png')
  })

  it('opens an image binary in the image diff viewer', async () => {
    const files: GitDiff[] = [
      {
        ...textFile(0),
        file_path: 'assets/logo.png',
        new_blob_base64: 'aGVsbG8=',
      },
    ]
    const outline: FileOutline[] = [
      { path: 'assets/logo.png', hunks: [], isPartial: false, isBinary: true },
    ]

    renderSurface(files, outline)

    fireEvent.click(screen.getByRole('button', { name: /assets\/logo\.png/i }))

    const image = await screen.findByAltText(/^assets\/logo\.png/)
    expect(image).toHaveAttribute('src', expect.stringContaining('aGVsbG8='))
  })
})

describe('ReviewCodeView scroll retention', () => {
  it('restores the scroll offset when the tab is switched away and back', () => {
    const { files, outline } = manyFiles(5)

    const first = renderSurface(files, outline)
    const scroller = scrollerOf(first.container)
    scroller.scrollTop = 400
    fireEvent.scroll(scroller)
    first.unmount()

    const second = renderSurface(files, outline)
    expect(scrollerOf(second.container).scrollTop).toBe(400)
  })

  it('keeps a commit diff tab at a separate offset from the branch review on the same workspace', () => {
    const { files, outline } = manyFiles(5)
    const store = createWorkspaceStore('ws-1')

    const branch = render(
      <WorkspaceStoreContext.Provider value={store}>
        <ReviewCodeView wsId="ws-1" files={files} outline={outline} />
      </WorkspaceStoreContext.Provider>,
    )
    const branchScroller = scrollerOf(branch.container)
    branchScroller.scrollTop = 300
    fireEvent.scroll(branchScroller)
    branch.unmount()

    const commitView = render(
      <WorkspaceStoreContext.Provider value={store}>
        <ReviewCodeView wsId="ws-1" commit="abc123" files={files} outline={outline} />
      </WorkspaceStoreContext.Provider>,
    )
    expect(scrollerOf(commitView.container).scrollTop).toBe(0)
  })
})
