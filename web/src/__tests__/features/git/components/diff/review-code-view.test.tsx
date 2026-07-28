import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  buildPlaceholderFileDiff,
  partitionReviewFiles,
  ReviewCodeView,
  REVIEW_TOKENIZE_MAX_LENGTH,
  REVIEW_TOKENIZE_MAX_LINE_LENGTH,
} from '@/features/git/components/diff/review-code-view'
import type { FileOutline } from '@/features/git/api/review-window-api'
import type { GitDiff } from '@/features/git/types/git-types'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'

/**
 * Task B of the windowed Branch Review renderer
 * (docs/superpowers/plans/2026-07-27-diff-perf-phase-3-renderer.md).
 *
 * The surface under test reserves scroll space for EVERY changed file from the
 * outline's hunk geometry, then holds the patch text of only the handful near
 * the viewport. So the assertions here are about what is *not* fetched as much
 * as what is.
 *
 * Two traps this file is written around, both paid for already:
 *
 *  - jsdom has no layout, so a virtualised surface measures 0×0 and renders
 *    nothing. The `getBoundingClientRect` stub below (same pattern as
 *    changed-files-tree.scale.test.tsx) is what makes the window real.
 *  - the file header renders almost immediately and SURVIVES a broken render,
 *    so waiting on non-empty shadow text passes against a component that
 *    rendered no code at all. Every render assertion here blocks on a CODE
 *    marker that only exists inside a real patch body.
 */

const mocks = vi.hoisted(() => ({
  getReviewPatch: vi.fn(),
}))

vi.mock('@/features/git/api/review-window-api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/features/git/api/review-window-api')>()
  return { ...actual, getReviewPatch: mocks.getReviewPatch }
})

// ── jsdom geometry ──────────────────────────────────────────────────

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

// ── Fixtures ────────────────────────────────────────────────────────

const pathOf = (i: number) => `src/pkg/file${i}.ts`

/** A marker that exists ONLY inside a patch body, never in a file header. */
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

/**
 * Render the surface inside a workspace store, which is where the review pane
 * always lives — the image viewer's shared header reads the active buffer from
 * it to implement its close button.
 */
function renderSurface(files: GitDiff[], outline: FileOutline[]) {
  const store = createWorkspaceStore('ws-1')
  const view = render(
    <WorkspaceStoreContext.Provider value={store}>
      <ReviewCodeView wsId="ws-1" files={files} outline={outline} />
    </WorkspaceStoreContext.Provider>,
  )
  return {
    ...view,
    /** Re-render as a refreshed summary would, without remounting the surface. */
    update(nextFiles: GitDiff[], nextOutline: FileOutline[]) {
      view.rerender(
        <WorkspaceStoreContext.Provider value={store}>
          <ReviewCodeView wsId="ws-1" files={nextFiles} outline={nextOutline} />
        </WorkspaceStoreContext.Provider>,
      )
    },
  }
}

/** Text of every rendered diff's shadow root, joined. */
function shadowText(container: HTMLElement): string {
  return [...container.querySelectorAll('diffs-container')]
    .map((host) => (host as HTMLElement).shadowRoot?.textContent ?? '')
    .join('\n')
}

/** Block until a CODE marker reaches the DOM — never merely until text exists. */
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

/**
 * Total scroll space the surface has reserved, in pixels.
 *
 * CodeView sizes its own scaffold element to the summed height of every item,
 * so this is the one honest read of "did the file reserve the right space" —
 * the file header's ± counts are cached by the library and do not refresh.
 */
function reservedHeight(container: HTMLElement): number {
  const scaffold = scrollerOf(container).firstElementChild as HTMLElement | null
  return Number.parseFloat(scaffold?.style.height || '0')
}

// ── Pure geometry ───────────────────────────────────────────────────

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
    // @@ -1,3 +1,3 @@ with ±1 is 2 context rows + 1 removal + 1 addition.
    expect(fileDiff.unifiedLineCount).toBe(4)
    expect(fileDiff.splitLineCount).toBe(3)
  })

  it('tops a PARTIAL file up from the summary counts, since its outline is a lower bound', () => {
    // The server caps the outline at 1000 hunks per file, so a partial file's
    // hunks describe only the beginning of the diff. Sizing from them alone
    // under-reserves scroll space by however much the cap cut.
    const file: GitDiff = { ...textFile(0), additions: 40_000, deletions: 20_000 }
    const capped: FileOutline = { ...textOutline(0), isPartial: true }

    const partial = buildPlaceholderFileDiff(file, capped)
    const complete = buildPlaceholderFileDiff(file, { ...capped, isPartial: false })

    expect(partial.unifiedLineCount).toBeGreaterThan(complete.unifiedLineCount)
    // Every changed line renders one unified row, so the ± counts are a floor.
    expect(partial.unifiedLineCount).toBeGreaterThanOrEqual(60_000)
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

// ── The surface ─────────────────────────────────────────────────────

describe('ReviewCodeView windowing', () => {
  it('renders the head of the diff and fetches only the files near it', async () => {
    const { files, outline } = manyFiles(60)
    const { container } = renderSurface(files, outline)

    await waitForCode(container, markerOf(0))

    const fetched = fetchedPaths()
    expect(fetched).toContain(pathOf(0))
    // The tail of a 60-file review is nowhere near the viewport.
    expect(fetched).not.toContain(pathOf(59))
    expect(new Set(fetched).size).toBeLessThan(files.length)
  })

  // Scroll-driven windowing is NOT asserted at this level, deliberately.
  //
  // The visible band comes from the renderer's own getRenderedItems(), which is
  // the only source that cannot disagree with what is on screen. jsdom's
  // renderer does not re-window on a simulated scroll, so that set never
  // changes here and any "scrolling fetches new files" assertion would be
  // testing the stub, not the component.
  //
  // Where it IS covered: planWindow's band/eviction/budget logic in
  // patch-window.test.ts (pure, exhaustive), and live verification — on a
  // 406-file / 1M-line review, jumping the scrollbar fetches the file on
  // screen first and RSS stays flat at ~728MB across the whole branch.
  it('materialises the files in view on first paint, and only those', async () => {
    const { files, outline } = manyFiles(200)
    const { container } = renderSurface(files, outline)

    await waitForCode(container, markerOf(0))

    const fetched = fetchedPaths()
    expect(fetched).toContain(pathOf(0))
    // Bounded: a 200-file review does not fetch 200 patches to paint.
    expect(fetched.length).toBeLessThan(60)
  })

  it('re-reserves space when a refreshed summary resizes a file still on placeholder', async () => {
    // The summary refetches on every git-status tick, so a file can grow while
    // the pane is open. Only its placeholder carries the reserved height, and
    // the file list is unchanged — so there is no remount to rebuild it.
    const { files, outline } = manyFiles(3)
    mocks.getReviewPatch.mockImplementation(async (_wsId: string, path: string) => {
      // File 0 never resolves, so it stays on its placeholder — which is
      // exactly the case under test.
      if (path === pathOf(0)) return new Promise<never>(() => {})
      return { patch: patchFor(path), truncated: false }
    })

    const { container, update } = renderSurface(files, outline)
    // A sibling file's code proves the surface really rendered.
    await waitForCode(container, markerOf(1))
    const before = reservedHeight(container)
    expect(before).toBeGreaterThan(0)

    update(
      files.map((f, i) => (i === 0 ? { ...f, additions: 400, deletions: 0 } : f)),
      outline.map((o, i) =>
        i === 0 ? { ...o, hunks: [{ oldStart: 1, oldLines: 0, newStart: 1, newLines: 400 }] } : o,
      ),
    )

    // 4 reserved rows became 400, at the library's 20px default row height.
    await waitFor(() => {
      expect(reservedHeight(container) - before).toBeGreaterThan(396 * 20 * 0.9)
    })
  })

  it('reserves MORE scroll space for a partial file than its outline alone describes', async () => {
    // The rendered counterpart of the buildPlaceholderFileDiff unit above: it
    // proves the topped-up geometry actually reaches the virtualiser, rather
    // than merely being computed and dropped.
    const file: GitDiff = { ...textFile(0), additions: 40_000, deletions: 20_000 }
    // Never resolves: the assertion must be about the PLACEHOLDER's reservation.
    mocks.getReviewPatch.mockImplementation(async () => new Promise<never>(() => {}))

    const complete = renderSurface([file], [textOutline(0)])
    const partial = renderSurface([file], [{ ...textOutline(0), isPartial: true }])

    await waitFor(() => {
      expect(reservedHeight(complete.container)).toBeGreaterThan(0)
      expect(reservedHeight(partial.container)).toBeGreaterThan(0)
    })
    expect(reservedHeight(partial.container)).toBeGreaterThan(
      reservedHeight(complete.container) * 100,
    )
  })

  // Eviction semantics are NOT asserted here, deliberately.
  //
  // They belong to planWindow, which is pure and covered exhaustively in
  // patch-window.test.ts — including the eviction band, the budgets, and the
  // hysteresis that stops a boundary file thrashing. At this level the band
  // comes from the renderer, and jsdom's renderer does not window the way a
  // real one does: it reports far more items as rendered than a viewport
  // holds, so a jsdom "scroll away" never actually takes a file out of range.
  // A test asserting eviction here would be asserting a jsdom artefact.
  //
  // The real behaviour is verified live instead: scrolling a 406-file /
  // 1M-line review end to end held webview RSS flat at ~728MB and it fell to
  // 634MB at the far end, which is eviction doing its job.
})

describe('ReviewCodeView truncated patches', () => {
  const MONSTER = 'dist/bundle.js'

  // The fixture's monster file is a SINGLE 420k-line hunk: under the server's
  // default cap NOTHING fits, so the response is a header-only patch. It must
  // read as "show all", never as an empty file.
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
    // Nothing of the file's body arrived — that is exactly the case that must
    // not read as "this file has no changes".
    expect(shadowText(container)).not.toContain('crowbarMonsterMarker')

    fireEvent.click(showAll)

    await waitForCode(container, 'crowbarMonsterMarker')
    expect(mocks.getReviewPatch).toHaveBeenCalledWith('ws-1', MONSTER, 0)
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
    // A retry must not escalate to the uncapped read; that would answer a
    // failed fetch of an ordinary file with the most expensive request there is.
    fireEvent.click(retry)

    await waitForCode(container, markerOf(0))
    expect(mocks.getReviewPatch).not.toHaveBeenCalledWith('ws-1', pathOf(0), 0)
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

    // ImageDiffViewer labels each side of a modified image; the blob we gave it
    // is the new one, so its <img> is what proves the viewer actually mounted.
    const image = await screen.findByAltText(/^assets\/logo\.png/)
    expect(image).toHaveAttribute('src', expect.stringContaining('aGVsbG8='))
  })
})
