import { createElement } from 'react'
import { act, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import {
  WorkspaceStoreContext,
  useWorkspaceStoreContext,
} from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'

// AgentChatPane is a heavy PTY-attached surface (xterm, WebSocket attach, its own
// revive state machine). This test is about HOSTING — is every chat kept mounted,
// and is only the inactive one hidden — not about chat behavior, so stub it to a
// passive marker that records the visibility/focus props threaded from the pane.
// Mocked via the SAME '@/...' id pane-container lazy-imports, so the mock intercepts
// it (the pattern agent-chat-pane.test.tsx / pane-container-suspense.test.tsx rely on).
vi.mock('@/features/agent/components/agent-chat-pane', () => ({
  AgentChatPane: ({
    chatId,
    isVisible,
    isActivePane,
  }: {
    chatId: string
    isVisible: boolean
    isActivePane: boolean
  }) =>
    createElement('div', {
      'data-testid': `chat-${chatId}`,
      'data-chat-id': chatId,
      'data-visible': String(isVisible),
      'data-active-pane': String(isActivePane),
    }),
}))

// TabBar drags in SidebarProvider/dnd-kit machinery irrelevant to hosting.
vi.mock('@/features/tabs/components/tab-bar', () => ({
  default: () => null,
}))

import { PaneContainer } from '@/features/panes/components/pane-container'

function PaneHost() {
  const pane = useWorkspaceStoreContext((s) => s.panes[ROOT_PANE_ID])
  if (!pane) return null
  return createElement(PaneContainer, { pane })
}

/** Open two agent-chat buffers into the root pane. The SECOND is the active tab
 *  (openContent's addBufferToPane(..., true) activates it), so the first is the
 *  hidden keep-alive one. */
function seedStore() {
  const store = createWorkspaceStore('w1')
  const hidden = store.getState().bufferActions.openContent({
    type: 'agentChat',
    chatId: 'cHidden',
    wsId: 'w1',
    name: 'Hidden chat',
    runnerId: 'rH',
  })
  const active = store.getState().bufferActions.openContent({
    type: 'agentChat',
    chatId: 'cActive',
    wsId: 'w1',
    name: 'Active chat',
    runnerId: 'rA',
  })
  return { store, hidden, active }
}

async function renderPane(store: ReturnType<typeof seedStore>['store']) {
  await act(async () => {
    render(createElement(WorkspaceStoreContext.Provider, { value: store }, createElement(PaneHost)))
  })
}

describe('PaneContainer agent-chat keep-alive hosting', () => {
  it('mounts every agent chat, hiding all but the active tab', async () => {
    const { store } = seedStore()
    await renderPane(store)

    // BOTH chats are mounted — that is the keep-alive: switching tabs must not remount
    // a live PTY. Getting the hidden one at all (not queryBy) is the assertion.
    const hidden = await screen.findByTestId('chat-cHidden')
    const active = await screen.findByTestId('chat-cActive')

    // The inactive chat's wrapper is HIDDEN (mounted, not displayed); the active one
    // has no such inline style. The wrapper is the marker's parent — <Suspense> adds
    // no DOM node between them.
    const hiddenWrapper = hidden.parentElement
    const activeWrapper = active.parentElement
    expect(hiddenWrapper).toHaveStyle({ visibility: 'hidden' })
    expect(activeWrapper).not.toHaveStyle({ visibility: 'hidden' })
    // Both share the absolute-inset overlay slot.
    expect(hiddenWrapper?.className).toContain('absolute')
    expect(activeWrapper?.className).toContain('absolute')
  })

  it('threads isVisible = (is the active tab) to each chat', async () => {
    const { store } = seedStore()
    await renderPane(store)

    // isVisible is the ACTIVE-TAB flag, independent of pane focus.
    expect((await screen.findByTestId('chat-cActive')).getAttribute('data-visible')).toBe('true')
    expect((await screen.findByTestId('chat-cHidden')).getAttribute('data-visible')).toBe('false')
  })

  it('switching the active tab flips visibility without unmounting the terminals', async () => {
    const { store, hidden } = seedStore()
    await renderPane(store)

    const hiddenBefore = await screen.findByTestId('chat-cHidden')
    const activeBefore = await screen.findByTestId('chat-cActive')
    expect(hiddenBefore.getAttribute('data-visible')).toBe('false')

    // Activate the previously-hidden tab.
    await act(async () => {
      store.getState().paneActions.activatePaneBuffer(ROOT_PANE_ID, hidden)
    })

    // Same DOM nodes — neither chat remounted — but visibility swapped.
    const hiddenAfter = screen.getByTestId('chat-cHidden')
    const activeAfter = screen.getByTestId('chat-cActive')
    expect(hiddenAfter).toBe(hiddenBefore)
    expect(activeAfter).toBe(activeBefore)
    expect(hiddenAfter.getAttribute('data-visible')).toBe('true')
    expect(activeAfter.getAttribute('data-visible')).toBe('false')
    expect(hiddenAfter.parentElement).not.toHaveStyle({ visibility: 'hidden' })
    expect(activeAfter.parentElement).toHaveStyle({ visibility: 'hidden' })
  })
})
