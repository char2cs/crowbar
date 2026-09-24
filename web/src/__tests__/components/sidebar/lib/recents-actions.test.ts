import { beforeEach, describe, expect, it, vi } from 'vitest'

const { openChatRoute } = vi.hoisted(() => ({ openChatRoute: vi.fn(() => true) }))
vi.mock('@/components/layout/space-content-actions', () => ({ openChatRoute }))
vi.mock('@/features/panes/lib/release-closed-chat', () => ({
  releaseClosedChat: vi.fn(async () => {}),
}))

import { closeRecent, closeRecentChat, focusRecent } from '@/components/sidebar/lib/recents-actions'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import { chatPaneIndex } from '@/features/panes/lib/view-selectors'
import { viewChatIds } from '@/features/panes/lib/view-state'
import type { Repo } from '@/lib/store/sidebar'

const repos: Repo[] = [
  {
    id: 'r1',
    projectId: 'p1',
    name: 'crowbar',
    avatarLabel: 'C',
    avatarColor: 'bg-indigo-700',
    workspaces: [{ id: 'ws-1', branch: 'main', age: '' }],
    chats: [
      { id: 'chat-a', repoId: 'r1', title: 'A', order: 0, workspaceId: 'ws-1' },
      { id: 'chat-b', repoId: 'r1', title: 'B', order: 1, workspaceId: 'ws-1' },
    ],
  },
]

function viewOf(chatId: string): string {
  const { panes } = windowPaneStore.getState()
  return panes[chatPaneIndex(panes).get(chatId)!].viewId!
}

beforeEach(() => {
  vi.clearAllMocks()
  resetWindowPaneStoreForTests()
  const { paneActions } = windowPaneStore.getState()
  paneActions.setActiveProject('p1')
  paneActions.openChat('chat-a')
  paneActions.openChat('chat-b')
})

describe('focusRecent', () => {
  it('switches to the row’s view, then routes to its chat’s own workspace', () => {
    const navigate = vi.fn()
    focusRecent(viewOf('chat-a'), repos, navigate as never)
    expect(windowPaneStore.getState().activeViewId).toBe(viewOf('chat-a'))
    expect(openChatRoute).toHaveBeenCalledWith(repos, 'chat-a', 'ws-1', navigate)
  })

  it('focuses the member of a group that was focused last', () => {
    const { paneActions } = windowPaneStore.getState()
    const paneA = chatPaneIndex(windowPaneStore.getState().panes).get('chat-a')!
    paneActions.dropChatOnPane('chat-b', paneA, 'right')
    paneActions.setActivePane(chatPaneIndex(windowPaneStore.getState().panes).get('chat-b')!)
    focusRecent(viewOf('chat-a'), repos, vi.fn() as never)
    expect(openChatRoute).toHaveBeenCalledWith(repos, 'chat-b', 'ws-1', expect.anything())
  })

  it('is a no-op for a row that no longer exists', () => {
    focusRecent('gone', repos, vi.fn() as never)
    expect(openChatRoute).not.toHaveBeenCalled()
  })
})

describe('closeRecent / closeRecentChat', () => {
  it('closeRecent ends the whole view — its row goes in one press', () => {
    const view = viewOf('chat-a')
    closeRecent(view)
    expect(windowPaneStore.getState().viewOrder).not.toContain(view)
    expect(chatPaneIndex(windowPaneStore.getState().panes).has('chat-a')).toBe(false)
  })

  it('closeRecentChat takes one member out of a group, leaving the rest grouped', () => {
    const { paneActions } = windowPaneStore.getState()
    paneActions.openChat('chat-c')
    const paneA = chatPaneIndex(windowPaneStore.getState().panes).get('chat-a')!
    paneActions.dropChatOnPane('chat-b', paneA, 'right')
    paneActions.dropChatOnPane('chat-c', paneA, 'bottom')
    const group = viewOf('chat-a')

    closeRecentChat('chat-b')

    expect(viewChatIds(windowPaneStore.getState(), group)).toEqual(['chat-a', 'chat-c'])
  })
})
