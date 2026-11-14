import { describe, expect, it } from 'vitest'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import type { PaneGroup } from '@/features/panes/types/pane'
import { getPaneScopeForPaneId } from '@/features/panes/utils/pane-routing'
import { createLeaf, createSplit } from '@/features/panes/utils/pane-layout'

function makeGroup(id: string, overrides: Partial<PaneGroup> = {}): PaneGroup {
  return {
    id,
    type: 'group',
    chatId: null,
    runnerId: null,
    editorTabIds: [],
    activeEditorTabId: null,
    editorOpen: true,
    viewId: null,
    ...overrides,
  }
}

describe('pane routing', () => {
  it('keeps routing scope within the active pane tree', () => {
    const rootPane = makeGroup(ROOT_PANE_ID)
    const bottomActivePane = makeGroup('bottom-active', {
      editorTabIds: ['terminal-a'],
      activeEditorTabId: 'terminal-a',
    })
    const bottomFallbackPane = makeGroup('bottom-fallback', {
      editorTabIds: ['terminal-b'],
      activeEditorTabId: 'terminal-b',
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
