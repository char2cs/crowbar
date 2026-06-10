import { describe, expect, it } from 'vitest'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import type { PaneGroup, LayoutNode } from '@/features/panes/types/pane'
import {
  getPaneScopeForPaneId,
  resolveWritablePaneForBuffer,
} from '@/features/panes/utils/pane-routing'
import { createLeaf, createSplit } from '@/features/panes/utils/pane-layout'

const BOTTOM_PANE_ID = 'bottom-pane'

function makeGroup(id: string, overrides: Partial<PaneGroup> = {}): PaneGroup {
  return { id, type: 'group', bufferIds: [], activeBufferId: null, ...overrides }
}

describe('pane routing', () => {
  it('keeps existing buffers in a locked active pane', () => {
    const activePane = makeGroup(ROOT_PANE_ID, { bufferIds: ['buffer-a'], locked: true })
    const panes = { [ROOT_PANE_ID]: activePane }
    const rootLayout = createLeaf(ROOT_PANE_ID)
    const bottomLayout = createLeaf(BOTTOM_PANE_ID)
    const paneScope = getPaneScopeForPaneId(rootLayout, bottomLayout, panes, ROOT_PANE_ID)

    expect(
      resolveWritablePaneForBuffer({
        activePane,
        bufferId: 'buffer-a',
        mostRecentActivePaneIds: [ROOT_PANE_ID],
        paneScope,
      }),
    ).toBe(activePane)
  })

  it('routes new buffers to the most recent unlocked pane in the same tree', () => {
    const activePane = makeGroup(ROOT_PANE_ID, { bufferIds: ['buffer-a'], locked: true })
    const firstFallback = makeGroup('pane-b', {
      bufferIds: ['buffer-b'],
      activeBufferId: 'buffer-b',
    })
    const mostRecentFallback = makeGroup('pane-c', {
      bufferIds: ['buffer-c'],
      activeBufferId: 'buffer-c',
    })
    const rootLayout: LayoutNode = createSplit(
      'horizontal',
      createLeaf(ROOT_PANE_ID),
      createSplit('vertical', createLeaf('pane-b'), createLeaf('pane-c')),
    )
    const bottomLayout = createLeaf(BOTTOM_PANE_ID)
    const panes = {
      [ROOT_PANE_ID]: activePane,
      'pane-b': firstFallback,
      'pane-c': mostRecentFallback,
    }
    const paneScope = getPaneScopeForPaneId(rootLayout, bottomLayout, panes, ROOT_PANE_ID)

    expect(
      resolveWritablePaneForBuffer({
        activePane,
        bufferId: 'buffer-d',
        mostRecentActivePaneIds: [activePane.id, mostRecentFallback.id, firstFallback.id],
        paneScope,
      }),
    ).toBe(mostRecentFallback)
  })

  it('returns null when every pane in the active tree is locked', () => {
    const activePane = makeGroup(ROOT_PANE_ID, { bufferIds: ['buffer-a'], locked: true })
    const lockedFallback = makeGroup('pane-b', { bufferIds: ['buffer-b'], locked: true })
    const rootLayout: LayoutNode = createSplit(
      'horizontal',
      createLeaf(ROOT_PANE_ID),
      createLeaf('pane-b'),
    )
    const bottomLayout = createLeaf(BOTTOM_PANE_ID)
    const panes = { [ROOT_PANE_ID]: activePane, 'pane-b': lockedFallback }
    const paneScope = getPaneScopeForPaneId(rootLayout, bottomLayout, panes, ROOT_PANE_ID)

    expect(
      resolveWritablePaneForBuffer({
        activePane,
        bufferId: 'buffer-c',
        mostRecentActivePaneIds: [lockedFallback.id, activePane.id],
        paneScope,
      }),
    ).toBeNull()
  })

  it('keeps routing scope within the active pane tree', () => {
    const rootPane = makeGroup(ROOT_PANE_ID)
    const bottomActivePane = makeGroup('bottom-active', {
      bufferIds: ['terminal-a'],
      activeBufferId: 'terminal-a',
    })
    const bottomFallbackPane = makeGroup('bottom-fallback', {
      bufferIds: ['terminal-b'],
      activeBufferId: 'terminal-b',
    })
    const rootLayout = createLeaf(ROOT_PANE_ID)
    const bottomLayout = createSplit(
      'horizontal',
      createLeaf('bottom-active'),
      createLeaf('bottom-fallback'),
    )
    const panes = {
      [ROOT_PANE_ID]: rootPane,
      'bottom-active': bottomActivePane,
      'bottom-fallback': bottomFallbackPane,
    }

    expect(getPaneScopeForPaneId(rootLayout, bottomLayout, panes, 'bottom-fallback')).toEqual([
      bottomActivePane,
      bottomFallbackPane,
    ])
  })
})
