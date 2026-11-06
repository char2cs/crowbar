import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import { useDocumentTitle } from '@/features/window/hooks/use-document-title'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import { resetChatStore, writeChat } from '@/__tests__/__fixtures__/agent-chat'

beforeEach(() => {
  resetChatStore()
  resetWindowPaneStoreForTests()
  document.title = 'Crowbar'
})

describe('useDocumentTitle', () => {
  it('names the focused chat in the window title, and follows its rename', () => {
    writeChat({ id: 'chat-1', workspaceId: 'ws-1', title: 'Add div' })
    windowPaneStore.getState().paneActions.openChat('chat-1', { workspaceId: 'ws-1' })
    renderHook(() => useDocumentTitle())
    expect(document.title).toBe('Add div — Crowbar')

    act(() => writeChat({ id: 'chat-1', title: 'Add div and mod' }))
    expect(document.title).toBe('Add div and mod — Crowbar')
  })

  it('is just the product name with no chat in focus', () => {
    renderHook(() => useDocumentTitle())
    expect(document.title).toBe('Crowbar')
  })
})
