import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'

// Trap 5 of the project-scoped panes design: the persistence subscription
// SHORT-CIRCUITS on a shallow compare of every field it knows about, so a new
// field left out of that comparison is a field whose changes never arm the
// debounce timer at all — it silently never persists, and every view comes
// back unfiled after a reload.
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

describe('a view’s project survives a reload', () => {
  it('opening a chat under a project persists that view’s tag', () => {
    const actions = windowPaneStore.getState().paneActions
    actions.setActiveProject('project-a')
    const paneId = actions.addPane()!
    actions.setPaneChat(paneId, 'chat-a', null)
    const viewId = windowPaneStore.getState().activeViewId

    vi.advanceTimersByTime(400)

    expect(mockSave).toHaveBeenCalled()
    expect(lastSavedLayout().viewProjects).toEqual({ [viewId]: 'project-a' })
  })

  it('the per-project last-showing view is persisted with it', () => {
    const actions = windowPaneStore.getState().paneActions
    actions.setActiveProject('project-a')
    const paneId = actions.addPane()!
    actions.setPaneChat(paneId, 'chat-a', null)
    const viewId = windowPaneStore.getState().activeViewId

    actions.setActiveProject('project-b')
    vi.advanceTimersByTime(400)

    expect(lastSavedLayout().activeViewByProject).toEqual({ 'project-a': viewId })
  })

  it('a tag change ALONE arms the timer — nothing else has to move', () => {
    windowPaneStore.setState({ viewProjects: { 'view-x': 'project-a' } })

    vi.advanceTimersByTime(400)

    expect(mockSave).toHaveBeenCalledTimes(1)
    expect(lastSavedLayout().viewProjects).toEqual({ 'view-x': 'project-a' })
  })

  it('activeProjectId is deliberately NOT persisted — the route says where now is', () => {
    windowPaneStore.setState({ activeProjectId: 'project-a', viewProjects: { v: 'project-a' } })

    vi.advanceTimersByTime(400)

    expect(lastSavedLayout()).not.toHaveProperty('activeProjectId')
  })
})
