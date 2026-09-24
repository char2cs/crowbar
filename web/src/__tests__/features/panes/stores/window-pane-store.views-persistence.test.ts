import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'

vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
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

describe('the band survives a reload', () => {
  it('opening chats persists the records, their order, the stage and the pointers', () => {
    const { paneActions } = windowPaneStore.getState()
    paneActions.setActiveProject('project-a')
    paneActions.openChat('chat-a')
    paneActions.openChat('chat-b')
    vi.advanceTimersByTime(300)

    const state = windowPaneStore.getState()
    const saved = lastSavedLayout()
    expect(saved.views).toEqual(state.views)
    expect(saved.viewOrder).toEqual(state.viewOrder)
    expect(saved.viewOrder).toHaveLength(2)
    expect(saved.activeViewId).toBe(state.activeViewId)
    expect(saved.activeViewByProject).toEqual({ 'project-a': state.activeViewId })
    expect(saved.stage).toEqual(state.stage)
  })

  it('a reorder ALONE arms the timer — nothing else has to move', () => {
    const { paneActions } = windowPaneStore.getState()
    paneActions.openChat('chat-a')
    paneActions.openChat('chat-b')
    vi.advanceTimersByTime(300)
    mockSave.mockClear()
    const [first, second] = windowPaneStore.getState().viewOrder

    paneActions.reorderView(second, first, 'before')
    vi.advanceTimersByTime(300)

    expect(mockSave).toHaveBeenCalledTimes(1)
    expect(lastSavedLayout().viewOrder).toEqual([second, first])
  })

  it('activeProjectId is deliberately NOT persisted — the route says where now is', () => {
    windowPaneStore.getState().paneActions.setActiveProject('project-a')
    windowPaneStore.getState().paneActions.openChat('chat-a')
    vi.advanceTimersByTime(300)
    expect(lastSavedLayout()).not.toHaveProperty('activeProjectId')
  })
})
