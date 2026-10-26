import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/features/agent/api/agent-api', () => ({
  closeChat: vi.fn().mockResolvedValue(undefined),
}))

import {
  cancelCloseRequest,
  confirmCloseRequest,
  requestClosePane,
  requestCloseView,
} from '@/features/panes/lib/guarded-close'
import { closeRequestStore } from '@/features/panes/stores/close-request-store'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import {
  destroyWorkspaceStore,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import { chatPaneIndex } from '@/features/panes/lib/view-selectors'

function openChat(chatId: string, working: boolean): string {
  getOrCreateWorkspaceStore('ws-1').setState((s) => ({
    agentChats: { ...s.agentChats, working: { ...s.agentChats.working, [chatId]: working } },
  }))
  windowPaneStore.getState().paneActions.openChat(chatId, { workspaceId: 'ws-1' })
  return chatPaneIndex(windowPaneStore.getState().panes).get(chatId) as string
}

const hasPane = (paneId: string) => paneId in windowPaneStore.getState().panes

beforeEach(() => {
  resetWindowPaneStoreForTests()
  cancelCloseRequest()
})

afterEach(() => {
  destroyWorkspaceStore('ws-1')
})

describe('guarded close of a chat pane', () => {
  it('closes an idle chat at once, without asking', () => {
    const paneId = openChat('idle-chat', false)

    requestClosePane(paneId)

    expect(hasPane(paneId)).toBe(false)
    expect(closeRequestStore.getState().request).toBeNull()
  })

  it('holds a working chat open and asks instead', () => {
    const paneId = openChat('busy-chat', true)

    requestClosePane(paneId)

    expect(hasPane(paneId)).toBe(true)
    expect(closeRequestStore.getState().request).toEqual({ kind: 'pane', id: paneId, working: 1 })
  })

  it('closes the working chat only once the user confirms', () => {
    const paneId = openChat('busy-chat', true)
    requestClosePane(paneId)

    confirmCloseRequest()

    expect(hasPane(paneId)).toBe(false)
    expect(closeRequestStore.getState().request).toBeNull()
  })

  it('keeps the working chat when the user cancels', () => {
    const paneId = openChat('busy-chat', true)
    requestClosePane(paneId)

    cancelCloseRequest()

    expect(hasPane(paneId)).toBe(true)
    expect(closeRequestStore.getState().request).toBeNull()
  })
})

describe('guarded close of a view', () => {
  it('asks when any chat in the view is working', () => {
    const paneId = openChat('busy-chat', true)
    const viewId = windowPaneStore.getState().panes[paneId].viewId as string

    requestCloseView(viewId)

    expect(windowPaneStore.getState().views[viewId]).toBeDefined()
    expect(closeRequestStore.getState().request).toEqual({ kind: 'view', id: viewId, working: 1 })
  })

  it('closes a view of idle chats at once', () => {
    const paneId = openChat('idle-chat', false)
    const viewId = windowPaneStore.getState().panes[paneId].viewId as string

    requestCloseView(viewId)

    expect(windowPaneStore.getState().views[viewId]).toBeUndefined()
  })
})
