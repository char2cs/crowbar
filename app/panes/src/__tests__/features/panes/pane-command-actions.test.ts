import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { resetWindowPaneStoreForTests } from '@/__tests__/__fixtures__/window-pane-store'
import { getOwningChatId } from '@/lib/workspace-scope'

vi.mock('@/lib/workspace-scope', () => ({
  getOwningChatId: vi.fn(),
}))

describe('ensurePaneChatThenOpen', () => {
  beforeEach(() => {
    resetWindowPaneStoreForTests()
    vi.mocked(getOwningChatId).mockReset()
  })

  it('runs openTab directly when the pane already has a chat — never consults the owning chat', async () => {
    const { ensurePaneChatThenOpen } = await import('@/features/panes/utils/pane-command-actions')
    windowPaneStore.getState().paneActions.openChat('existing-chat', { runnerId: 'runner-1' })
    const openTab = vi.fn()

    ensurePaneChatThenOpen('ws-1', ROOT_PANE_ID, openTab)

    expect(openTab).toHaveBeenCalledExactlyOnceWith(ROOT_PANE_ID, 'ws-1')
    expect(getOwningChatId).not.toHaveBeenCalled()
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe('existing-chat')
  })

  it("attaches the workspace's real owning chat to a chatless pane, then opens", async () => {
    const { ensurePaneChatThenOpen } = await import('@/features/panes/utils/pane-command-actions')
    vi.mocked(getOwningChatId).mockReturnValue('owning-chat-1')
    const openTab = vi.fn()

    ensurePaneChatThenOpen('ws-1', ROOT_PANE_ID, openTab)

    expect(getOwningChatId).toHaveBeenCalledWith('ws-1')
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe('owning-chat-1')
    expect(openTab).toHaveBeenCalledExactlyOnceWith(ROOT_PANE_ID, 'ws-1')
  })

  it('does nothing — no chat attached, openTab never runs — when no owning chat resolves', async () => {
    const { ensurePaneChatThenOpen } = await import('@/features/panes/utils/pane-command-actions')
    vi.mocked(getOwningChatId).mockReturnValue(null)
    const openTab = vi.fn()

    ensurePaneChatThenOpen('ws-1', ROOT_PANE_ID, openTab)

    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBeNull()
    expect(openTab).not.toHaveBeenCalled()
  })

  it('reveals a pane already showing the owning chat rather than duplicating it into this one', async () => {
    const { ensurePaneChatThenOpen } = await import('@/features/panes/utils/pane-command-actions')
    const paneActions = windowPaneStore.getState().paneActions
    const otherPaneId = paneActions.splitPane(ROOT_PANE_ID, 'horizontal')
    if (!otherPaneId) throw new Error('split failed')
    paneActions.dropChatOnPane('owning-chat-1', otherPaneId, 'center')
    paneActions.setActivePane(ROOT_PANE_ID)
    vi.mocked(getOwningChatId).mockReturnValue('owning-chat-1')
    const openTab = vi.fn()

    ensurePaneChatThenOpen('ws-1', ROOT_PANE_ID, openTab)

    expect(windowPaneStore.getState().activePaneId).toBe(otherPaneId)
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBeNull()
    expect(openTab).toHaveBeenCalledExactlyOnceWith(otherPaneId, 'ws-1')
  })
})

describe('openBranchReviewForWorkspace', () => {
  beforeEach(() => {
    resetWindowPaneStoreForTests()
  })

  it('opens into the given paneId, not whichever pane was active before the call', async () => {
    const { openBranchReviewForWorkspace } =
      await import('@/features/panes/utils/pane-command-actions')
    const paneActions = windowPaneStore.getState().paneActions
    const otherPaneId = paneActions.splitPane(ROOT_PANE_ID, 'horizontal')
    if (!otherPaneId) throw new Error('split failed')
    paneActions.setActivePane(ROOT_PANE_ID)

    const bufferId = openBranchReviewForWorkspace('ws-1', otherPaneId)

    expect(bufferId).not.toBeNull()
    expect(windowPaneStore.getState().activePaneId).toBe(ROOT_PANE_ID)
    expect(windowPaneStore.getState().panes[otherPaneId]?.editorTabIds).toContain(bufferId)
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.editorTabIds).not.toContain(bufferId)
    const opened = windowPaneStore.getState().buffers.find((b) => b.id === bufferId)
    expect(opened?.workspaceId).toBe('ws-1')
  })

  it('with no paneId, keeps opening into whichever pane is already active (a caller with no pane of its own)', async () => {
    const { openBranchReviewForWorkspace } =
      await import('@/features/panes/utils/pane-command-actions')
    const paneActions = windowPaneStore.getState().paneActions
    const otherPaneId = paneActions.splitPane(ROOT_PANE_ID, 'horizontal')
    if (!otherPaneId) throw new Error('split failed')
    paneActions.setActivePane(otherPaneId)

    const bufferId = openBranchReviewForWorkspace('ws-1')

    expect(windowPaneStore.getState().activePaneId).toBe(otherPaneId)
    expect(windowPaneStore.getState().panes[otherPaneId]?.editorTabIds).toContain(bufferId)
  })

  it('returns null and touches nothing when wsId is null', async () => {
    const { openBranchReviewForWorkspace } =
      await import('@/features/panes/utils/pane-command-actions')
    const before = windowPaneStore.getState().activePaneId
    expect(openBranchReviewForWorkspace(null, 'some-pane')).toBeNull()
    expect(windowPaneStore.getState().activePaneId).toBe(before)
  })
})
