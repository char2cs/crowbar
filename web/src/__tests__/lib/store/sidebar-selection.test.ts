/**
 * The store half of the two-state keep model.
 *
 * `keep-set.ts` pins the RULES; this pins the thing that makes them safe to
 * apply — that the two sets never move together. A single write that touched
 * both is how "clear the selection" ends up wiping rows the user deliberately
 * kept, and nothing about that failure looks like a state bug when you see it:
 * the rows simply vanish when you click elsewhere.
 */
import { describe, expect, test, beforeEach } from 'vitest'
import { getInitialSelectionState, useSidebarSelectionStore } from '@/lib/store/sidebar-selection'

const state = () => useSidebarSelectionStore.getState()

beforeEach(() => {
  useSidebarSelectionStore.setState(getInitialSelectionState())
})

describe('the multiselection', () => {
  test('cmd-click adds a row and anchors on it', () => {
    state().toggleSelected('a')
    expect([...state().selected]).toEqual(['a'])
    expect(state().anchorId).toBe('a')
  })

  test('cmd-click again takes it back off', () => {
    state().toggleSelected('a')
    state().toggleSelected('a')
    expect(state().selected.size).toBe(0)
  })

  test('shift-click fills the visual range between the anchor and the row', () => {
    state().toggleSelected('b')
    state().selectRange('d', ['a', 'b', 'c', 'd', 'e'])
    expect([...state().selected].sort()).toEqual(['b', 'c', 'd'])
  })

  test('shift-click upwards spans the same rows', () => {
    state().toggleSelected('d')
    state().selectRange('b', ['a', 'b', 'c', 'd', 'e'])
    expect([...state().selected].sort()).toEqual(['b', 'c', 'd'])
  })

  test('shift-click with no usable anchor degrades to the one row', () => {
    state().selectRange('c', ['a', 'b', 'c'])
    expect([...state().selected]).toEqual(['c'])
  })

  test('a plain click clears it — that, not the styling, is what stops a crowd', () => {
    state().toggleSelected('a')
    state().toggleSelected('b')
    state().clearSelected('c')
    expect(state().selected.size).toBe(0)
    expect(state().anchorId).toBe('c')
  })
})

describe('the keep set is a different set, and stays one', () => {
  test('clearing the selection leaves it untouched — by identity, not by value', () => {
    state().keepRows(['kept-1', 'kept-2'])
    const before = state().kept

    state().toggleSelected('a')
    state().selectRange('b', ['a', 'b'])
    state().clearSelected('c')

    // Same object: nothing on the selection path so much as rebuilt it.
    expect(state().kept).toBe(before)
    expect([...state().kept].sort()).toEqual(['kept-1', 'kept-2'])
  })

  test('keeping rows leaves the multiselection untouched', () => {
    state().toggleSelected('a')
    const before = state().selected
    state().keepRows(['kept-1'])
    expect(state().selected).toBe(before)
  })

  test('releasing rows the set never held is a no-op', () => {
    state().keepRows(['kept-1'])
    const before = state().kept
    state().releaseRows(['nothing'])
    expect(state().kept).toBe(before)
  })

  test('setKept replaces the set outright — the fold-away control', () => {
    state().keepRows(['a', 'b'])
    state().setKept(new Set(['b']))
    expect([...state().kept]).toEqual(['b'])
  })
})
