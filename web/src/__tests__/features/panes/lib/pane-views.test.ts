import { describe, expect, it } from 'vitest'
import { panesInView, viewIdOf, viewIsShared } from '@/features/panes/lib/pane-views'
import type { PaneGroup } from '@/features/panes/types/pane'

function pane(id: string, viewId?: string): PaneGroup {
  return {
    id,
    type: 'group',
    chatId: null,
    runnerId: null,
    editorTabIds: [],
    activeEditorTabId: null,
    editorOpen: false,
    ...(viewId !== undefined && { viewId }),
  }
}

function record(...panes: PaneGroup[]): Record<string, PaneGroup> {
  return Object.fromEntries(panes.map((p) => [p.id, p]))
}

describe('viewIdOf', () => {
  it('answers the tagged view when there is one', () => {
    expect(viewIdOf(pane('p1', 'view-a'))).toBe('view-a')
  })

  // No migration, just a correct reading of the old shape: every pane in a
  // layout persisted before views existed was independent.
  it('reads an UNTAGGED pane as a view of its own, named by its own id', () => {
    expect(viewIdOf(pane('p1'))).toBe('p1')
  })

  it('never lets two untagged panes look like one view', () => {
    expect(viewIdOf(pane('p1'))).not.toBe(viewIdOf(pane('p2')))
  })
})

describe('panesInView', () => {
  it('collects every pane sharing the view, the subject included', () => {
    const panes = record(pane('p1', 'v'), pane('p2', 'v'), pane('p3', 'other'))
    expect(panesInView(panes, 'p1').map((p) => p.id)).toEqual(['p1', 'p2'])
  })

  // The whole reason a dissolving view needs no special case anywhere.
  it('a pane nobody merged with is a view of exactly one', () => {
    const panes = record(pane('p1', 'v'), pane('p2', 'other'))
    expect(panesInView(panes, 'p1').map((p) => p.id)).toEqual(['p1'])
  })

  it('is empty for a pane that does not exist', () => {
    expect(panesInView(record(pane('p1')), 'gone')).toEqual([])
  })
})

describe('viewIsShared', () => {
  it('is true only while another pane carries the same view', () => {
    expect(viewIsShared(record(pane('p1', 'v'), pane('p2', 'v')), 'p1')).toBe(true)
  })

  it('is false for a lone member — a group of one is ungrouped', () => {
    expect(viewIsShared(record(pane('p1', 'v'), pane('p2', 'w')), 'p1')).toBe(false)
  })

  it('is false for untagged panes, which are never each other', () => {
    expect(viewIsShared(record(pane('p1'), pane('p2')), 'p1')).toBe(false)
  })
})
