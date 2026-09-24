import { describe, it, expect } from 'vitest'
import { repairViewState } from '@/features/panes/lib/view-repair'
import { viewIntegrityViolations } from '@/features/panes/lib/view-integrity'
import { makePane, viewChatIds } from '@/features/panes/lib/view-state'
import { createLeaf, getAllLeafIds } from '@/features/panes/utils/pane-layout'
import { buildViewState, row } from '@/__tests__/__fixtures__/view-state'

const band = () =>
  buildViewState({
    views: [
      { id: 'v1', panes: [{ id: 'a', chatId: 'chat-a' }] },
      {
        id: 'v2',
        projectId: 'p2',
        panes: [
          { id: 'b', chatId: 'chat-b' },
          { id: 'c', chatId: 'chat-c' },
        ],
      },
      { id: 'v3', panes: [{ id: 'd', chatId: 'chat-d' }] },
    ],
    active: 'v2',
  })

describe('repairViewState', () => {
  it('returns a valid state unchanged in substance', () => {
    const state = band()
    const repaired = repairViewState(state)
    expect(viewIntegrityViolations(repaired)).toEqual([])
    expect(repaired.viewOrder).toEqual(['v1', 'v2', 'v3'])
    expect(repaired.views).toEqual(state.views)
    expect(repaired.panes).toEqual(state.panes)
    expect(repaired.activeViewId).toBe('v2')
    expect(repaired.activePaneId).toBe(state.activePaneId)
  })

  it('drops only the chatless record and keeps the rest in order', () => {
    const state = band()
    state.panes.d = { ...state.panes.d, chatId: null }
    const repaired = repairViewState(state)
    expect(viewIntegrityViolations(repaired)).toEqual([])
    expect(repaired.viewOrder).toEqual(['v1', 'v2'])
    expect(repaired.panes.d).toBeUndefined()
  })

  it('drops a leaf naming no pane but keeps its view when a chat survives', () => {
    const state = band()
    state.views.v2 = { ...state.views.v2, layout: row(['b', 'ghost', 'c']) }
    const repaired = repairViewState(state)
    expect(viewIntegrityViolations(repaired)).toEqual([])
    expect(viewChatIds(repaired, 'v2')).toEqual(['chat-b', 'chat-c'])
  })

  it('keeps the first pane of a chat held twice (Law 4)', () => {
    const state = band()
    state.panes.d = { ...state.panes.d, chatId: 'chat-a' }
    const repaired = repairViewState(state)
    expect(viewIntegrityViolations(repaired)).toEqual([])
    expect(repaired.viewOrder).toEqual(['v1', 'v2'])
    expect(viewChatIds(repaired, 'v1')).toEqual(['chat-a'])
  })

  it('drops a pane placed in two layouts from the later one', () => {
    const state = band()
    state.views.v3 = { ...state.views.v3, layout: row(['d', 'a']) }
    const repaired = repairViewState(state)
    expect(viewIntegrityViolations(repaired)).toEqual([])
    expect(getAllLeafIds(repaired.views.v3.layout)).toEqual(['d'])
    expect(getAllLeafIds(repaired.views.v1.layout)).toEqual(['a'])
  })

  it('repairs order: duplicates, unknown ids, and records missing from it', () => {
    const state = band()
    state.viewOrder = ['v2', 'nope', 'v2', 'v1']
    const repaired = repairViewState(state)
    expect(viewIntegrityViolations(repaired)).toEqual([])
    expect(repaired.viewOrder).toEqual(['v2', 'v1', 'v3'])
  })

  it('clears pointers that name dropped records or panes', () => {
    const state = band()
    state.panes.b = { ...state.panes.b, chatId: null }
    state.panes.c = { ...state.panes.c, chatId: null }
    state.fullscreenPaneId = 'b'
    state.activeViewByProject = { p2: 'v2', p1: 'v2' }
    const repaired = repairViewState(state)
    expect(viewIntegrityViolations(repaired)).toEqual([])
    expect(repaired.activeViewId).toBeNull()
    expect(repaired.activeViewByProject).toEqual({})
    expect(repaired.fullscreenPaneId).toBeNull()
    expect(getAllLeafIds(repaired.stage)).toContain(repaired.activePaneId)
  })

  it('rebuilds a missing stage or tray and drops a chat left on the stage', () => {
    const state = band()
    state.panes.s = makePane('s', null, { chatId: 'chat-s' })
    state.stage = createLeaf('s')
    state.bottomLayout = createLeaf('gone')
    const repaired = repairViewState(state)
    expect(viewIntegrityViolations(repaired)).toEqual([])
    expect(repaired.panes.s).toBeUndefined()
    expect(repaired.viewOrder).toEqual(['v1', 'v2', 'v3'])
  })

  it('does not mutate its input', () => {
    const state = band()
    state.panes.d = { ...state.panes.d, chatId: null }
    const snapshot = JSON.stringify(state)
    repairViewState(state)
    expect(JSON.stringify(state)).toBe(snapshot)
  })
})
