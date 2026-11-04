import { renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { createChat } = vi.hoisted(() => ({ createChat: vi.fn(() => Promise.resolve('chat-1')) }))
vi.mock('@/features/agent/api/agent-api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/agent/api/agent-api')>()),
  createChat,
}))

import { useLaunchDraft } from '@/components/layout/mobile/use-launch-draft'
import { isDraftChatId } from '@/features/agent/lib/draft-chat'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'

const navigate = vi.fn(() => Promise.resolve())

function activeChatId(): string | null {
  const { panes, activePaneId } = windowPaneStore.getState()
  return panes[activePaneId]?.chatId ?? null
}

beforeEach(() => {
  resetWindowPaneStoreForTests()
  windowPaneStore.setState({ activeProjectId: 'p1' })
  windowPaneStore
    .getState()
    .paneActions.openChat('last-chat', { projectId: 'p1', workspaceId: 'home-1' })
  navigate.mockClear()
  createChat.mockClear()
})
afterEach(() => resetWindowPaneStoreForTests())

describe('useLaunchDraft', () => {
  it('lands a cold launch on a blank draft in the project home, over the last chat', async () => {
    renderHook(() => useLaunchDraft('p1', 'home-1', navigate))
    await vi.waitFor(() => expect(isDraftChatId(activeChatId())).toBe(true))
    expect(createChat).not.toHaveBeenCalled()
  })

  it('waits for the home workspace to resolve, leaving the restored chat meanwhile', async () => {
    const { rerender } = renderHook(
      ({ ws }: { ws: string | null }) => useLaunchDraft('p1', ws, navigate),
      { initialProps: { ws: null as string | null } },
    )
    expect(activeChatId()).toBe('last-chat')
    rerender({ ws: 'home-1' })
    await vi.waitFor(() => expect(isDraftChatId(activeChatId())).toBe(true))
  })

  // A resume is a re-render, not a launch: what the user is looking at stays.
  it('opens the draft once per launch, not again on later renders', async () => {
    const { rerender } = renderHook(() => useLaunchDraft('p1', 'home-1', navigate))
    await vi.waitFor(() => expect(isDraftChatId(activeChatId())).toBe(true))
    windowPaneStore
      .getState()
      .paneActions.openChat('picked', { projectId: 'p1', workspaceId: 'home-1' })
    rerender()
    expect(activeChatId()).toBe('picked')
  })

  it('does nothing with no project to open a draft in', () => {
    renderHook(() => useLaunchDraft(null, null, navigate))
    expect(activeChatId()).toBe('last-chat')
  })
})
