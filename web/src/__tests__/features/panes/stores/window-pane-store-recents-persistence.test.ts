import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'

// REGRESSION (restyle v2): a dormant Recents row and the order the user
// dragged the band into were gone after a reload — only the views still open
// came back. `dormantArrangements` and `recentsOrder` were never written to
// the persisted WorkspaceLayout, contradicting spec §5.5 ("the view it was is
// remembered so the close is undoable") and §5.6 ("order in Recents is the
// user's; only a drag changes it").
vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/editor/stores/buffer-session-persistence', () => ({
  saveSessionToStore: vi.fn(),
}))

import { saveWorkspaceLayout } from '@/lib/persistence/workspace-layout'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import type { WorkspaceLayout } from '@/lib/persistence/schemas'

const mockSave = saveWorkspaceLayout as ReturnType<typeof vi.fn>

function lastSavedLayout(): WorkspaceLayout {
  return mockSave.mock.calls.at(-1)![0] as WorkspaceLayout
}

beforeEach(() => {
  vi.useFakeTimers()
  resetWindowPaneStoreForTests()
  vi.runOnlyPendingTimers()
  mockSave.mockClear()
})

afterEach(() => {
  vi.runOnlyPendingTimers()
  vi.useRealTimers()
})

describe('Recents survives a reload', () => {
  it('a change to the dormant arrangements is persisted with the layout', () => {
    windowPaneStore.setState({
      dormantArrangements: [
        { id: 'entry-a', chatIds: ['chat-a'], state: 'dormant' },
        { id: 'entry-b', chatIds: ['chat-b'], state: 'dormant' },
      ],
    })
    vi.advanceTimersByTime(400)
    mockSave.mockClear()

    windowPaneStore.getState().paneActions.forgetDormantArrangement('entry-a')
    vi.advanceTimersByTime(400)

    expect(mockSave).toHaveBeenCalledTimes(1)
    expect(lastSavedLayout().dormantArrangements).toEqual([
      { id: 'entry-b', chatIds: ['chat-b'], state: 'dormant' },
    ])
  })

  it("a Recents drag persists the user's order", () => {
    windowPaneStore
      .getState()
      .paneActions.reorderRecentsEntry('entry-c', 'entry-a', 'before', [
        'entry-a',
        'entry-b',
        'entry-c',
      ])
    vi.advanceTimersByTime(400)

    expect(mockSave).toHaveBeenCalledTimes(1)
    expect(lastSavedLayout().recentsOrder).toEqual(['entry-c', 'entry-a', 'entry-b'])
  })
})
