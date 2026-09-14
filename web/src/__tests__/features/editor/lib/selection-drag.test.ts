import { afterEach, describe, expect, it } from 'vitest'
import {
  __resetSelectionDragForTests,
  beginSelectionDrag,
  endSelectionDrag,
  isSelectionDragging,
  releaseSelectionDrag,
} from '@/features/editor/lib/selection-drag'

const attr = () => document.documentElement.getAttribute('data-editor-selecting')

describe('selection-drag', () => {
  afterEach(() => {
    __resetSelectionDragForTests()
  })

  it('publishes the drag as both a flag and the CSS attribute', () => {
    expect(isSelectionDragging()).toBe(false)
    expect(attr()).toBeNull()

    beginSelectionDrag()
    expect(isSelectionDragging()).toBe(true)
    expect(attr()).toBe('1')

    endSelectionDrag()
    expect(isSelectionDragging()).toBe(false)
    expect(attr()).toBeNull()
  })

  // A window holds several editor panes. A second pane starting a drag while the
  // first is still holding one must not clear the flag (and the CALayer
  // promotion that rides on it) out from under the first.
  it('refcounts overlapping drags from separate panes', () => {
    beginSelectionDrag()
    beginSelectionDrag()
    endSelectionDrag()

    expect(isSelectionDragging()).toBe(true)
    expect(attr()).toBe('1')

    endSelectionDrag()
    expect(isSelectionDragging()).toBe(false)
    expect(attr()).toBeNull()
  })

  it('ignores an end with no drag in flight', () => {
    endSelectionDrag()
    endSelectionDrag()
    expect(isSelectionDragging()).toBe(false)
    expect(attr()).toBeNull()

    // …and is still able to start one afterwards (the count never went negative).
    beginSelectionDrag()
    expect(attr()).toBe('1')
    endSelectionDrag()
    expect(attr()).toBeNull()
  })

  // A pane unmounted mid-drag would otherwise pin the attribute on forever.
  it('releaseSelectionDrag drops only a claim the caller actually held', () => {
    beginSelectionDrag()
    releaseSelectionDrag(false)
    expect(isSelectionDragging()).toBe(true)

    releaseSelectionDrag(true)
    expect(isSelectionDragging()).toBe(false)
    expect(attr()).toBeNull()
  })
})
