// LAW 4 of the project-scoped panes design, at the call site §6.5 names:
// "no drop, no click, no command may put chat X into a view of a project that
// is not X's." The user's own words: "I shouldn't be able to move a view into
// another project."
import { describe, expect, it, beforeEach, vi } from 'vitest'

vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/editor/stores/buffer-session-persistence', () => ({
  saveSessionToStore: vi.fn(),
  clearQueuedWorkspaceSessionSave: vi.fn(),
}))
vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: vi.fn(), info: vi.fn(), success: vi.fn() },
}))
// The chat→project walk itself is exercised in chat-project's own tests; what
// matters here is which answer makes the drop land and which makes it refuse.
const { resolveChatProjectId } = vi.hoisted(() => ({ resolveChatProjectId: vi.fn() }))
vi.mock('@/features/panes/lib/chat-project', () => ({
  resolveChatProjectId: (...args: unknown[]) => resolveChatProjectId(...args),
}))

import { toast } from '@/features/window/stores/toast-store'
import { openChatIntoPane } from '@/components/sidebar/lib/drop-actions'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import { showingLayout } from '@/features/panes/lib/view-state'
import { chatPaneIndex } from '@/features/panes/lib/view-selectors'
import { getAllLeafIds } from '@/features/panes/utils/pane-layout'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

const chatRow = (id: string, wsId: string): SidebarRow => ({
  id,
  kind: 'chat',
  parentId: null,
  order: 0,
  label: id,
  ownsWorktree: false,
  workspaceId: wsId,
  working: false,
  hasView: false,
})

/** A view of `project`, on screen, holding `chatId`. Returns its pane id. */
function showChatUnderProject(project: string, chatId: string): string {
  const actions = windowPaneStore.getState().paneActions
  actions.setActiveProject(project)
  actions.openChat(chatId, { projectId: project })
  return chatPaneIndex(windowPaneStore.getState().panes).get(chatId)!
}

beforeEach(() => {
  resetWindowPaneStoreForTests()
  vi.mocked(toast.error).mockClear()
  resolveChatProjectId.mockReset()
})

describe('openChatIntoPane — law 4', () => {
  it("refuses a chat whose project is not the target view's", () => {
    const paneId = showChatUnderProject('project-a', 'chat-a')
    resolveChatProjectId.mockReturnValue('project-b')

    openChatIntoPane(chatRow('chat-b', 'ws-b'), paneId, 'right')

    expect(getAllLeafIds(showingLayout(windowPaneStore.getState()))).toEqual([paneId])
    expect(toast.error).toHaveBeenCalledWith('That chat belongs to a different space')
  })

  it('lets a chat of the SAME project through — the refusal is not a blanket', () => {
    const paneId = showChatUnderProject('project-a', 'chat-a')
    resolveChatProjectId.mockReturnValue('project-a')

    openChatIntoPane(chatRow('chat-b', 'ws-b'), paneId, 'right')

    const leaves = getAllLeafIds(showingLayout(windowPaneStore.getState()))
    expect(leaves).toHaveLength(2)
    expect(toast.error).not.toHaveBeenCalled()
  })

  it('lets an UNRESOLVABLE chat through rather than refusing on a guess', () => {
    // The sidebar is routinely a frame behind. Refusing on "don't know" would
    // break ordinary same-project drops, and the geometry already makes a
    // genuine cross-project drop nearly unreachable.
    const paneId = showChatUnderProject('project-a', 'chat-a')
    resolveChatProjectId.mockReturnValue(null)

    openChatIntoPane(chatRow('chat-b', 'ws-b'), paneId, 'right')

    expect(getAllLeafIds(showingLayout(windowPaneStore.getState()))).toHaveLength(2)
    expect(toast.error).not.toHaveBeenCalled()
  })
})
