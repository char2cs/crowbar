import { describe, it, expect } from 'vitest'
import {
  commitViewWrite,
  fillPane,
  insertPane,
  movePane,
  removePane,
  removeView,
} from '@/features/panes/lib/view-ops'
import { makePane, showingLayout, viewChatIds } from '@/features/panes/lib/view-state'
import { assertViewIntegrity, viewIntegrityViolations } from '@/features/panes/lib/view-integrity'
import { getAllLeafIds } from '@/features/panes/utils/pane-layout'
import { buildViewState } from '@/__tests__/__fixtures__/view-state'

const twoViews = () =>
  buildViewState({
    views: [
      { id: 'v1', panes: [{ id: 'a', chatId: 'chat-a' }] },
      {
        id: 'v2',
        panes: [
          { id: 'b', chatId: 'chat-b' },
          { id: 'c', chatId: 'chat-c' },
        ],
      },
    ],
    active: 'v1',
    activeProjectId: 'p1',
  })

describe('insertPane', () => {
  it('a new record is appended to viewOrder with the pane as its only leaf', () => {
    const state = twoViews()
    const viewId = insertPane(state, makePane('d', null, { chatId: 'chat-d' }), {
      kind: 'view',
      projectId: 'p1',
    })
    expect(viewId).toBeTruthy()
    expect(state.viewOrder).toEqual(['v1', 'v2', viewId])
    expect(state.views[viewId!].layout).toEqual({ type: 'pane', id: 'd' })
    expect(state.panes.d.viewId).toBe(viewId)
    assertViewIntegrity(state)
  })

  it('places a record directly after `after`', () => {
    const state = twoViews()
    const viewId = insertPane(state, makePane('d', null, { chatId: 'chat-d' }), {
      kind: 'view',
      projectId: 'p1',
      after: 'v1',
    })
    expect(state.viewOrder).toEqual(['v1', viewId, 'v2'])
  })

  it('refuses a chatless pane as a record of its own (invariant 2)', () => {
    const state = twoViews()
    expect(
      insertPane(state, makePane('d', null), { kind: 'view', projectId: 'p1' }),
    ).toBeUndefined()
    expect(state.panes.d).toBeUndefined()
  })

  it('a split joins the target view', () => {
    const state = twoViews()
    const joined = insertPane(state, makePane('d', null, { chatId: 'chat-d' }), {
      kind: 'split',
      targetPaneId: 'a',
      direction: 'horizontal',
      placement: 'after',
    })
    expect(joined).toBe('v1')
    expect(viewChatIds(state, 'v1')).toEqual(['chat-a', 'chat-d'])
    assertViewIntegrity(state)
  })

  it('a chat split into the stage promotes the stage into a record', () => {
    const state = buildViewState({ views: [], active: null, activeProjectId: 'p1' })
    const joined = insertPane(state, makePane('d', null, { chatId: 'chat-d' }), {
      kind: 'split',
      targetPaneId: 'root-pane',
      direction: 'horizontal',
      placement: 'after',
    })
    expect(joined).toBeTruthy()
    expect(state.views[joined!].projectId).toBe('p1')
    expect(getAllLeafIds(state.views[joined!].layout)).toEqual(['root-pane', 'd'])
    expect(state.activeViewId).toBe(joined)
    expect(getAllLeafIds(state.stage)).toHaveLength(1)
    expect(state.panes[getAllLeafIds(state.stage)[0]].chatId).toBeNull()
    assertViewIntegrity(state)
  })

  it('never puts a chat in the bottom tray', () => {
    const state = twoViews()
    const joined = insertPane(state, makePane('d', null, { chatId: 'chat-d' }), {
      kind: 'split',
      targetPaneId: 'bottom-pane',
      direction: 'vertical',
      placement: 'after',
    })
    expect(joined).toBeUndefined()
    expect(state.panes.d).toBeUndefined()
  })
})

describe('removePane', () => {
  it('the last chat leaving a view removes the record in the same step', () => {
    const state = twoViews()
    removePane(state, 'a')
    expect(state.views.v1).toBeUndefined()
    expect(state.viewOrder).toEqual(['v2'])
    // The showing view ended: the next view of the project comes forward.
    expect(state.activeViewId).toBe('v2')
    assertViewIntegrity(state)
  })

  // The route names the workspace on screen and only a gesture changes it: a
  // close must not bring another workspace's view forward under that route.
  it('the view that comes forward shows the same workspace, else the stage', () => {
    const state = buildViewState({
      views: [
        { id: 'v1', panes: [{ id: 'a', chatId: 'chat-a', workspaceId: 'ws-a' }] },
        { id: 'v2', panes: [{ id: 'b', chatId: 'chat-b', workspaceId: 'ws-b' }] },
        { id: 'v3', panes: [{ id: 'c', chatId: 'chat-c', workspaceId: 'ws-a' }] },
      ],
      active: 'v1',
      activeProjectId: 'p1',
    })
    state.mostRecentActivePaneIds = ['a', 'b', 'c']
    removePane(state, 'a')
    expect(state.activeViewId).toBe('v3')
    removePane(state, 'c')
    expect(state.activeViewId).toBeNull()
    expect(state.views.v2).toBeDefined()
    assertViewIntegrity(state)
  })

  it('a view keeps its record while another chat remains', () => {
    const state = twoViews()
    removePane(state, 'b')
    expect(viewChatIds(state, 'v2')).toEqual(['chat-c'])
    assertViewIntegrity(state)
  })

  it('a view left with only chatless panes is removed with them', () => {
    const state = buildViewState({
      views: [
        {
          id: 'v1',
          panes: [
            { id: 'a', chatId: 'chat-a' },
            { id: 'e', editorTabIds: ['tab-1'] },
          ],
        },
      ],
      activeProjectId: 'p1',
    })
    removePane(state, 'a')
    expect(state.views.v1).toBeUndefined()
    expect(state.panes.e).toBeUndefined()
    expect(state.activeViewId).toBeNull()
    assertViewIntegrity(state)
  })

  it('hands the leaving pane’s tabs to the survivor', () => {
    const state = buildViewState({
      views: [
        {
          id: 'v1',
          panes: [
            { id: 'a', chatId: 'chat-a', editorTabIds: ['t1'] },
            { id: 'b', chatId: 'chat-b', editorTabIds: ['t2'] },
          ],
        },
      ],
    })
    removePane(state, 'b')
    expect(state.panes.a.editorTabIds).toEqual(['t1', 't2'])
  })

  it('emptying the stage reseeds it with one empty pane', () => {
    const state = buildViewState({ views: [], active: null })
    removePane(state, 'root-pane')
    const leaves = getAllLeafIds(state.stage)
    expect(leaves).toHaveLength(1)
    expect(state.panes[leaves[0]].editorTabIds).toEqual([])
    assertViewIntegrity(state)
  })

  it('emptying the bottom tray reseeds it', () => {
    const state = twoViews()
    removePane(state, 'bottom-pane')
    expect(getAllLeafIds(state.bottomLayout)).toEqual(['bottom-pane'])
    assertViewIntegrity(state)
  })
})

describe('movePane', () => {
  it('a merge that takes the source view’s last chat removes that view', () => {
    const state = twoViews()
    const moved = movePane(state, 'a', {
      kind: 'split',
      targetPaneId: 'b',
      direction: 'horizontal',
      placement: 'before',
    })
    expect(moved).toBe(true)
    expect(state.views.v1).toBeUndefined()
    expect(viewChatIds(state, 'v2')).toEqual(['chat-a', 'chat-b', 'chat-c'])
    expect(state.panes.a.viewId).toBe('v2')
    assertViewIntegrity(state)
  })

  it('a merge leaves a multi-chat source view standing', () => {
    const state = twoViews()
    movePane(state, 'c', {
      kind: 'split',
      targetPaneId: 'a',
      direction: 'horizontal',
      placement: 'after',
    })
    expect(viewChatIds(state, 'v1')).toEqual(['chat-a', 'chat-c'])
    expect(viewChatIds(state, 'v2')).toEqual(['chat-b'])
    assertViewIntegrity(state)
  })

  it('moving within one view re-tiles it', () => {
    const state = twoViews()
    movePane(state, 'c', {
      kind: 'split',
      targetPaneId: 'b',
      direction: 'vertical',
      placement: 'before',
    })
    expect(viewChatIds(state, 'v2')).toEqual(['chat-c', 'chat-b'])
    assertViewIntegrity(state)
  })

  it('a move into a new record lands after `after`, same project', () => {
    const state = twoViews()
    expect(movePane(state, 'c', { kind: 'view', projectId: 'p1', after: 'v1' })).toBe(true)
    const newId = state.panes.c.viewId!
    expect(state.viewOrder).toEqual(['v1', newId, 'v2'])
    expect(viewChatIds(state, 'v2')).toEqual(['chat-b'])
    assertViewIntegrity(state)
  })

  it('a chat moved onto the stage promotes it', () => {
    const state = buildViewState({
      views: [{ id: 'v1', panes: [{ id: 'a', chatId: 'chat-a' }] }],
      active: null,
      activeProjectId: 'p1',
    })
    movePane(state, 'a', {
      kind: 'split',
      targetPaneId: 'root-pane',
      direction: 'horizontal',
      placement: 'after',
    })
    expect(state.views.v1).toBeUndefined()
    const viewId = state.panes.a.viewId!
    expect(getAllLeafIds(state.views[viewId].layout)).toEqual(['root-pane', 'a'])
    assertViewIntegrity(state)
  })
})

describe('fillPane', () => {
  it('filling the stage promotes it — its layout becomes the record’s', () => {
    const state = buildViewState({ views: [], active: null, activeProjectId: 'p1' })
    const viewId = fillPane(state, 'root-pane', 'chat-a', null, 'p1')
    expect(viewId).toBe('root-pane')
    expect(state.views['root-pane'].layout).toEqual({ type: 'pane', id: 'root-pane' })
    expect(state.activeViewId).toBe('root-pane')
    expect(state.stage).not.toEqual({ type: 'pane', id: 'root-pane' })
    assertViewIntegrity(state)
  })

  it('filling a chatless pane in a view joins that view', () => {
    const state = buildViewState({
      views: [
        {
          id: 'v1',
          panes: [
            { id: 'a', chatId: 'chat-a' },
            { id: 'e', editorTabIds: ['t'] },
          ],
        },
      ],
    })
    expect(fillPane(state, 'e', 'chat-e', null, 'p1')).toBe('v1')
    expect(viewChatIds(state, 'v1')).toEqual(['chat-a', 'chat-e'])
  })

  it('never replaces a chat (invariant 5)', () => {
    const state = twoViews()
    expect(fillPane(state, 'a', 'chat-z', null, 'p1')).toBeUndefined()
    expect(state.panes.a.chatId).toBe('chat-a')
  })
})

describe('removeView', () => {
  it('drops the record, its panes and any pointer to it', () => {
    const state = twoViews()
    removeView(state, 'v1')
    expect(state.views.v1).toBeUndefined()
    expect(state.panes.a).toBeUndefined()
    expect(state.activeViewByProject.p1).toBe('v2')
    assertViewIntegrity(state)
  })

  it('the last view of the project leaves the stage showing', () => {
    const state = buildViewState({ views: [{ id: 'v1', panes: [{ id: 'a', chatId: 'x' }] }] })
    removeView(state, 'v1')
    expect(state.activeViewId).toBeNull()
    expect(getAllLeafIds(showingLayout(state))).toContain(state.activePaneId)
    assertViewIntegrity(state)
  })
})

describe('viewIntegrityViolations', () => {
  it('flags a record with no chat, a chat in two panes and a stage chat', () => {
    const state = twoViews()
    state.panes.a.chatId = null
    state.panes.b.chatId = 'chat-c'
    state.panes['root-pane'].chatId = 'chat-s'
    const text = viewIntegrityViolations(state).join('\n')
    expect(text).toMatch(/view v1 holds no chat/)
    expect(text).toMatch(/chat chat-c is in panes/)
    expect(text).toMatch(/stage pane root-pane holds chat/)
  })

  it('flags viewOrder drift and a pane in no layout', () => {
    const state = twoViews()
    state.viewOrder = ['v1']
    state.panes.orphan = makePane('orphan', null)
    const text = viewIntegrityViolations(state).join('\n')
    expect(text).toMatch(/view v2 missing from viewOrder/)
    expect(text).toMatch(/pane orphan is in no layout/)
  })
})

describe('commitViewWrite — focus is derived from pane writes', () => {
  it('focus never names a removed pane, whichever op removed it', () => {
    for (const op of [
      (s: ReturnType<typeof twoViews>) => removePane(s, 'b'),
      (s: ReturnType<typeof twoViews>) => removeView(s, 'v2'),
      (s: ReturnType<typeof twoViews>) =>
        movePane(s, 'b', { kind: 'view', projectId: 'p1', after: 'v2' }),
    ]) {
      const state = twoViews()
      state.activeViewId = 'v2'
      state.activePaneId = 'b'
      commitViewWrite(state, op)
      expect(state.panes[state.activePaneId]).toBeDefined()
      assertViewIntegrity(state)
    }
  })
})
