import { act, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { FileExplorerTree } from '@/features/file-explorer/file-explorer/components/file-explorer-tree'
import type { FileEntry } from '@/features/file-system/types/app'

/**
 * **Can the reference be driven into `selected` programmatically?**
 *
 * This is the question the `file-tree-row` gate surface stands on. The native
 * side takes `--flags selected` and paints `var(--accent)`; if the React side
 * could only be selected by a *trusted* pointer event, the whole state axis
 * would be undrivable from a bridge and the surface would be no better than the
 * git status row it replaces.
 *
 * It cannot be answered by rendering `FileExplorerTreeItem` with `isActive`:
 * that proves the row *renders* the state, not that the app can be put into it.
 * The selection lives two components up — `FileExplorerTree` owns `focusedPath`
 * and `hasTreeFocus`, and the click is handled by **delegation on the scroll
 * container**, not by a handler on the row. So the real tree is mounted here.
 *
 * jsdom has no layout, so `@tanstack/react-virtual` measures the container at
 * 0×0 and renders no rows at all. The one stub below gives it a height; nothing
 * about the selection path is stubbed.
 */

const FILES: FileEntry[] = [
  { name: 'a.ts', path: '/repo/a.ts', isDir: false },
  { name: 'b.ts', path: '/repo/b.ts', isDir: false },
  { name: 'c.ts', path: '/repo/c.ts', isDir: false },
]

const VIEWPORT = { width: 294, height: 600 }

let originalRect: typeof Element.prototype.getBoundingClientRect

beforeEach(() => {
  originalRect = Element.prototype.getBoundingClientRect
  Element.prototype.getBoundingClientRect = function boundingRect(this: Element) {
    const rect = originalRect.call(this) as DOMRect
    return {
      ...rect,
      width: VIEWPORT.width,
      height: VIEWPORT.height,
      right: VIEWPORT.width,
      bottom: VIEWPORT.height,
      toJSON: rect.toJSON,
    } as DOMRect
  }
})

afterEach(() => {
  Element.prototype.getBoundingClientRect = originalRect
})

function mountTree() {
  const result = render(
    <FileExplorerTree
      files={FILES}
      rootFolderPath="/repo"
      onFileSelect={vi.fn()}
      onCreateNewFileInDirectory={vi.fn()}
    />,
  )
  const rows = () =>
    Array.from(result.container.querySelectorAll<HTMLElement>('[data-oracle-id="file-row-item"]'))
  const selected = () => rows().filter((row) => row.getAttribute('data-active') === 'true')
  const button = (index: number) =>
    rows()[index].querySelector<HTMLElement>('[data-oracle-id="file-row-button"]')

  return { ...result, rows, selected, button }
}

describe('driving the file-tree-row surface into `selected`', () => {
  it('renders one anchored row per file, none of them selected at rest', () => {
    const tree = mountTree()

    expect(tree.rows()).toHaveLength(FILES.length)
    expect(tree.selected()).toHaveLength(0)
  })

  /**
   * **The recipe, and the answer to "does a programmatic click work".**
   *
   * It does, and it takes two steps rather than one. `handleContainerClick`
   * writes `focusedPath`, but the tree only *reads* `focusedPath` while it has
   * focus — `highlightedPath = hasTreeFocus ? (focusedPath || activePath) :
   * activePath` — and an effect resets `focusedPath` to `activePath` whenever it
   * does not. So a click on its own selects nothing unless the owning app also
   * moves `activePath`, which means opening the file.
   *
   * Focusing the container first is what makes the click self-contained. Both
   * events are ordinary DOM events dispatched from script; neither is gated on
   * `isTrusted`, and React's delegated listener at the root receives a bubbling
   * synthetic click exactly as it receives a real one.
   */
  it('selects a row from a focus + bubbling click, with no trusted pointer input', () => {
    const tree = mountTree()
    const container = tree.container.querySelector<HTMLElement>('.file-tree-container')
    expect(container).not.toBeNull()

    act(() => {
      container?.focus()
    })
    act(() => {
      tree.button(1)?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })

    expect(tree.selected()).toHaveLength(1)
    expect(tree.selected()[0]).toBe(tree.rows()[1])
    expect(tree.rows()[1].getAttribute('data-active')).toBe('true')
    expect(tree.rows()[0].getAttribute('data-active')).toBeNull()
  })

  /**
   * The click may land anywhere inside the row, because `getTargetItem` walks up
   * to the nearest `[data-file-path]` — which is the button. Worth pinning: an
   * extractor-driven run will most naturally click the anchor it already knows,
   * and the name span is a child of the button rather than the button itself.
   */
  it('accepts a click on the name span, not only on the button', () => {
    const tree = mountTree()
    const container = tree.container.querySelector<HTMLElement>('.file-tree-container')
    const name = tree.rows()[2].querySelector<HTMLElement>('[data-oracle-id="file-row-name"]')

    act(() => {
      container?.focus()
    })
    act(() => {
      name?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })

    expect(tree.selected()[0]).toBe(tree.rows()[2])
  })

  /**
   * Focus alone already selects — the container's `onFocusCapture` seeds
   * `focusedPath` with `activePath || visibleRows[0]`. Recorded because it is a
   * cheaper way to reach a `selected` cell than clicking, **and** because it is
   * the trap in the other direction: a run that focuses the tree to drive
   * `focus` gets `selected` for free and would be comparing the wrong cell if it
   * did not say so.
   */
  it('selects the first row on focus alone, which is a state a `focus` run enters by accident', () => {
    const tree = mountTree()
    const container = tree.container.querySelector<HTMLElement>('.file-tree-container')

    act(() => {
      container?.focus()
    })

    expect(tree.selected()).toHaveLength(1)
    expect(tree.selected()[0]).toBe(tree.rows()[0])
  })

  /**
   * `activePath` is the other way in, and the one that needs no focus at all:
   * it is a prop, so an app that has opened a file has a selected row whether or
   * not the tree is focused. Cheapest possible drive for a `selected` cell.
   */
  it('selects from the activePath prop with no interaction whatsoever', () => {
    const tree = render(
      <FileExplorerTree
        files={FILES}
        rootFolderPath="/repo"
        activePath="/repo/c.ts"
        onFileSelect={vi.fn()}
        onCreateNewFileInDirectory={vi.fn()}
      />,
    )

    const rows = Array.from(
      tree.container.querySelectorAll<HTMLElement>('[data-oracle-id="file-row-item"]'),
    )
    expect(rows.filter((row) => row.getAttribute('data-active') === 'true')).toEqual([rows[2]])
  })

  /**
   * **`focus` is a different matter, and this is the honest finding.**
   *
   * The `:focus-visible` border the native row models belongs to the row
   * *button*, and the tree never gives a button DOM focus: the container holds
   * focus and moves a virtual cursor with `aria-activedescendant`. So the state
   * is reachable only by focusing the button directly, out of band with the
   * tree's own interaction model — and even then `:focus-visible` is a
   * heuristic on how focus arrived, which jsdom does not implement at all.
   *
   * What is pinned here is only what jsdom can honestly answer: the button *is*
   * focusable and takes DOM focus, and the tree's own navigation does not put it
   * there. Whether `WebKit` then matches `:focus-visible` has to be measured in
   * the real engine.
   */
  it('never gives a row button DOM focus through the tree, though the button is focusable', () => {
    const tree = mountTree()
    const container = tree.container.querySelector<HTMLElement>('.file-tree-container')

    act(() => {
      container?.focus()
    })
    act(() => {
      tree.button(1)?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    // Selected, and yet the container is what holds focus.
    expect(tree.selected()).toHaveLength(1)
    expect(document.activeElement).toBe(container)
    expect(container?.getAttribute('aria-activedescendant')).toBeTruthy()

    // The button will take focus if it is asked directly — which is the only
    // way to reach the `:focus-visible` rule.
    act(() => {
      tree.button(1)?.focus()
    })
    expect(document.activeElement).toBe(tree.button(1))
  })
})
