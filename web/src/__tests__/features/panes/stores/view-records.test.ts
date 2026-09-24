import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/panes/lib/release-closed-chat', () => ({
  releaseClosedChat: vi.fn(async () => {}),
}))

import {
  createWindowPaneStore,
  type WindowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import { viewMembers } from '@/features/panes/lib/view-state'
import { chatPaneIndex } from '@/features/panes/lib/view-selectors'
import { releaseClosedChat } from '@/features/panes/lib/release-closed-chat'

/** Record-carried identity: C1 (views change only on gestures) and C3
 *  (a member's project and workspace are fixed at open). */
describe('view records carry their members’ identity', () => {
  let store: WindowPaneStore
  const actions = () => store.getState().paneActions
  const paneOf = (chatId: string) => chatPaneIndex(store.getState().panes).get(chatId)!

  beforeEach(() => {
    store = createWindowPaneStore()
    actions().setActiveProject('p1')
  })

  it('every opening gesture records the member’s workspace', () => {
    actions().openChat('c1', { workspaceId: 'ws-a' })
    actions().dropChatOnPane('c2', paneOf('c1'), 'right', 'ws-b')
    actions().adoptBackgroundChat('c3', 'p1', 'ws-c')

    const [grouped, background] = store.getState().viewOrder
    expect(viewMembers(store.getState(), grouped)).toEqual([
      { chatId: 'c1', workspaceId: 'ws-a' },
      { chatId: 'c2', workspaceId: 'ws-b' },
    ])
    expect(viewMembers(store.getState(), background)).toEqual([
      { chatId: 'c3', workspaceId: 'ws-c' },
    ])
  })

  it('a runner move changes the chat a pane follows, never the views or the workspace (C1, C3)', () => {
    actions().openChat('c1', { workspaceId: 'ws-a', runnerId: 'r1' })
    const before = store.getState().viewOrder
    actions().retargetPane(paneOf('c1'), 'c1-next', 'r1')

    expect(store.getState().viewOrder).toEqual(before)
    expect(viewMembers(store.getState(), before[0])).toEqual([
      { chatId: 'c1-next', workspaceId: 'ws-a' },
    ])
  })

  it('a project switch creates and deletes no view (C1)', () => {
    actions().openChat('c1', { workspaceId: 'ws-a', projectId: 'p1' })
    actions().openChat('c2', { workspaceId: 'ws-b', projectId: 'p2' })
    const views = Object.keys(store.getState().views).sort()

    actions().setActiveProject('p2')
    actions().setActiveProject('p1')

    expect(Object.keys(store.getState().views).sort()).toEqual(views)
  })

  it('closing a view releases each member against its recorded workspace', () => {
    actions().openChat('c1', { workspaceId: 'ws-a' })
    actions().dropChatOnPane('c2', paneOf('c1'), 'right', 'ws-b')
    vi.mocked(releaseClosedChat).mockClear()

    actions().closeView(store.getState().viewOrder[0])

    const calls = vi.mocked(releaseClosedChat).mock.calls.map(([chatId, ws]) => [chatId, ws])
    expect(calls).toEqual([
      ['c1', 'ws-a'],
      ['c2', 'ws-b'],
    ])
  })
})
