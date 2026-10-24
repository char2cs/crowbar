import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { IDBFactory } from 'fake-indexeddb'
import { createRef } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  ReviewCodeView,
  type ReviewCodeViewHandle,
} from '@/features/git/components/diff/review-code-view'
import type { FileOutline } from '@/features/git/api/review-window-api'
import { resetDB } from '@/lib/persistence/idb'
import type { GitDiff } from '@/features/git/types/git-types'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { markViewed } from '@/__tests__/__fixtures__/viewed'

// Fixtures mirror review-code-view.test.tsx: jsdom has no layout, so the rect
// stub makes the virtualised surface real, and every render assertion blocks
// on a CODE marker that only exists inside a patch body.

const mocks = vi.hoisted(() => ({ getReviewPatch: vi.fn(), getReviewFileSides: vi.fn() }))

vi.mock('@/features/git/api/review-window-api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/features/git/api/review-window-api')>()
  return {
    ...actual,
    getReviewPatch: mocks.getReviewPatch,
    getReviewFileSides: mocks.getReviewFileSides,
  }
})

const originalGetBoundingClientRect = HTMLElement.prototype.getBoundingClientRect

beforeEach(() => {
  globalThis.indexedDB = new IDBFactory()
  resetDB()
  const rect = { top: 0, left: 0, right: 900, bottom: 800, width: 900, height: 800, x: 0, y: 0 }
  HTMLElement.prototype.getBoundingClientRect = function getBoundingClientRect() {
    return { ...rect, toJSON: () => rect } as DOMRect
  }
  mocks.getReviewPatch.mockReset()
  mocks.getReviewFileSides.mockReset()
  mocks.getReviewPatch.mockImplementation(async (_scope: unknown, path: string) => ({
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

function manyFiles(count: number): { files: GitDiff[]; outline: FileOutline[] } {
  return {
    files: Array.from({ length: count }, (_, i) => ({
      file_path: pathOf(i),
      is_new: false,
      is_deleted: false,
      is_renamed: false,
      lines: [],
      additions: 1,
      deletions: 1,
    })),
    outline: Array.from({ length: count }, (_, i) => ({
      path: pathOf(i),
      hunks: [{ oldStart: 1, oldLines: 3, newStart: 1, newLines: 3 }],
      isPartial: false,
      isBinary: false,
    })),
  }
}

async function renderSurface(files: GitDiff[], outline: FileOutline[], viewed: string[] = []) {
  const store = createWorkspaceStore('ws-1')
  await markViewed(
    store,
    files.filter((f) => viewed.includes(f.file_path)),
  )
  const tree = (nextFiles: GitDiff[]) => (
    <WorkspaceStoreContext.Provider value={store}>
      <ReviewCodeView wsId="ws-1" files={nextFiles} outline={outline} />
    </WorkspaceStoreContext.Provider>
  )
  const view = render(tree(files))
  return { ...view, update: (nextFiles: GitDiff[]) => view.rerender(tree(nextFiles)) }
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

function reservedHeight(container: HTMLElement): number {
  const scroller = container.querySelector('[data-review-code-view-scroller]') as HTMLElement
  const scaffold = scroller.firstElementChild as HTMLElement | null
  return Number.parseFloat(scaffold?.style.height || '0')
}

describe('review patch window with viewed files', () => {
  it('never fetches a viewed file, though it is first on screen, and reserves no rows for it', async () => {
    const { files, outline } = manyFiles(3)
    const plain = await renderSurface(files, outline)
    await waitForCode(plain.container, markerOf(0))
    const openHeight = reservedHeight(plain.container)
    plain.unmount()
    mocks.getReviewPatch.mockClear()

    const { container } = await renderSurface(files, outline, [pathOf(0)])
    await waitForCode(container, markerOf(1))

    expect(fetchedPaths()).not.toContain(pathOf(0))
    expect(shadowText(container)).not.toContain(markerOf(0))
    expect(reservedHeight(container)).toBeLessThan(openHeight)
  })

  it('collapses and evicts a held file when it is checked, and re-fetches it when unchecked', async () => {
    const { files, outline } = manyFiles(3)
    const { container } = await renderSurface(files, outline)
    await waitForCode(container, markerOf(0))

    const [first] = screen.getAllByRole('checkbox', { name: /viewed/i })
    fireEvent.click(first)
    await waitFor(() => expect(shadowText(container)).not.toContain(markerOf(0)))
    mocks.getReviewPatch.mockClear()

    fireEvent.click(screen.getAllByRole('checkbox', { name: /viewed/i })[0])
    await waitForCode(container, markerOf(0))
    expect(fetchedPaths()).toContain(pathOf(0))
  })

  it('reveals a viewed file by scrolling to its header without fetching it', async () => {
    const { files, outline } = manyFiles(3)
    const handle = createRef<ReviewCodeViewHandle>()
    const store = createWorkspaceStore('ws-1')
    await markViewed(store, [files[2]])
    const { container } = render(
      <WorkspaceStoreContext.Provider value={store}>
        <ReviewCodeView wsId="ws-1" files={files} outline={outline} surfaceRef={handle} />
      </WorkspaceStoreContext.Provider>,
    )
    await waitForCode(container, markerOf(0))
    mocks.getReviewPatch.mockClear()

    act(() => {
      handle.current?.revealFile(pathOf(2))
      handle.current?.revealLine(pathOf(2), 1, 'new')
    })

    expect(fetchedPaths()).not.toContain(pathOf(2))
  })

  it('shows a file as unviewed once its summary changes', async () => {
    const { files, outline } = manyFiles(2)
    const { container, update } = await renderSurface(files, outline, [pathOf(0)])
    await waitForCode(container, markerOf(1))
    expect(fetchedPaths()).not.toContain(pathOf(0))

    update([{ ...files[0], additions: 9 }, files[1]])
    await waitForCode(container, markerOf(0))
  })

  it("keeps a viewed file's header counts and its Viewed checkbox after it collapses", async () => {
    const { files, outline } = manyFiles(3)
    const { container } = await renderSurface(files, outline)
    await waitForCode(container, markerOf(0))

    fireEvent.click(screen.getAllByRole('checkbox', { name: /viewed/i })[0])
    await waitFor(() => expect(shadowText(container)).not.toContain(markerOf(0)))

    const [host] = container.querySelectorAll('diffs-container')
    const root = (host as HTMLElement).shadowRoot as ShadowRoot
    expect(root.querySelector('[data-additions-count]')?.textContent).toBe('+1')
    expect(root.querySelector('[data-deletions-count]')?.textContent).toBe('-1')
    expect(screen.getAllByRole('checkbox', { name: /viewed/i })).toHaveLength(3)
  })

  it("fetches a file's full text only when a separator is expanded, then reveals the lines", async () => {
    const lines = (edit: string) =>
      Array.from({ length: 30 }, (_, i) => (i === 10 ? edit : `unchangedLine${i + 1}`)).join('\n')
    mocks.getReviewFileSides.mockResolvedValue({ old: lines('before'), new: lines('after') })
    mocks.getReviewPatch.mockImplementation(async (_scope: unknown, path: string) => ({
      patch: `diff --git a/${path} b/${path}
index 1111111..2222222 100644
--- a/${path}
+++ b/${path}
@@ -10,3 +10,3 @@ ${markerOf(0)}
 unchangedLine10
-before
+after
 unchangedLine12
`,
      truncated: false,
    }))
    const { files, outline } = manyFiles(1)
    const { container } = await renderSurface(files, outline)
    await waitForCode(container, 'unchangedLine10')
    expect(mocks.getReviewFileSides).not.toHaveBeenCalled()
    expect(shadowText(container)).not.toContain('unchangedLine3')

    const [host] = container.querySelectorAll('diffs-container')
    const expand = (host as HTMLElement).shadowRoot?.querySelector('[data-expand-button]')
    fireEvent.click(expand as Element)

    await waitForCode(container, 'unchangedLine3')
    expect(mocks.getReviewFileSides).toHaveBeenCalledTimes(1)
    expect(mocks.getReviewFileSides).toHaveBeenCalledWith(
      { wsId: 'ws-1', commit: undefined },
      pathOf(0),
      undefined,
    )
  })
})
