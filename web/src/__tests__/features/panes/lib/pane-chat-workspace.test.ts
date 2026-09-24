import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))

import {
  resolveChatWorkspaceId,
  resolveOnscreenPaneForWorkspace,
} from '@/features/panes/lib/pane-chat-workspace'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import { chatPaneIndex } from '@/features/panes/lib/view-selectors'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'

const paneActions = () => windowPaneStore.getState().paneActions

beforeEach(() => {
  resetWindowPaneStoreForTests()
})

/**
 * `resolveChatWorkspaceId` — "which workspace does this chat belong to". A
 * chat on screen answers from its view member's record (C3); anything else
 * from the caller's own row. No workspace store is consulted.
 */
describe('resolveChatWorkspaceId', () => {
  it('answers from the pane record the opening gesture wrote', () => {
    paneActions().openChat('c1', { workspaceId: 'ws-a' })
    expect(resolveChatWorkspaceId('c1')).toBe('ws-a')
  })

  it('prefers the record over the caller’s hint when the two disagree', () => {
    paneActions().openChat('c1', { workspaceId: 'ws-a' })
    expect(resolveChatWorkspaceId('c1', 'ws-other')).toBe('ws-a')
  })

  it('falls back to the hint for a chat not on screen', () => {
    expect(resolveChatWorkspaceId('c1', 'ws-a')).toBe('ws-a')
  })

  it('is null when nothing can name a workspace for the chat', () => {
    expect(resolveChatWorkspaceId('c-ghost')).toBeNull()
  })
})

describe('resolveOnscreenPaneForWorkspace', () => {
  it('returns null when the active pane already belongs to the target workspace', () => {
    paneActions().openChat('c1', { workspaceId: 'ws-a' })
    paneActions().setActivePane(ROOT_PANE_ID)

    expect(resolveOnscreenPaneForWorkspace('ws-a')).toBeNull()
  })

  it('names the on-screen sibling pane that belongs to the target workspace', () => {
    paneActions().openChat('c1', { workspaceId: 'ws-a' })
    paneActions().dropChatOnPane('c2', ROOT_PANE_ID, 'right', 'ws-b')
    const secondPaneId = chatPaneIndex(windowPaneStore.getState().panes).get('c2')
    // The user's last literal click landed in the ws-a pane...
    paneActions().setActivePane(ROOT_PANE_ID)

    // ...but the file explorer is showing ws-b: target the OTHER on-screen pane.
    expect(resolveOnscreenPaneForWorkspace('ws-b')).toBe(secondPaneId)
  })

  it('returns null when no on-screen pane belongs to the target workspace', () => {
    paneActions().openChat('c1', { workspaceId: 'ws-a' })
    paneActions().setActivePane(ROOT_PANE_ID)

    expect(resolveOnscreenPaneForWorkspace('ws-nobody-showing')).toBeNull()
  })

  it('never targets a pane sitting in a PARKED (off-screen) view', () => {
    paneActions().openChat('c2', { workspaceId: 'ws-b' })
    // A second chat takes the screen as a view of its own; the ws-b pane is
    // off screen, and a file click must never reveal it.
    paneActions().openChat('c1', { workspaceId: 'ws-a' })

    expect(resolveOnscreenPaneForWorkspace('ws-b')).toBeNull()
  })
})
