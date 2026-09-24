import { act, cleanup, render } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/editor/stores/buffer-session-persistence', () => ({
  saveSessionToStore: vi.fn(),
  clearQueuedWorkspaceSessionSave: vi.fn(),
}))

import { useActivePaneWorkspaceId } from '@/features/panes/hooks/use-chat-workspace-id'
import {
  destroyWorkspaceStore,
  getAllActiveWorkspaceIds,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'

const repoWithChat = (chatId: string, wsId: string): Repo => ({
  id: 'repo-1',
  name: 'repo-1',
  avatarLabel: 'R',
  avatarColor: '#000000',
  workspaces: [],
  chats: [{ id: chatId, repoId: 'repo-1', title: chatId, order: 0, workspaceId: wsId }],
})

afterEach(() => {
  cleanup()
  getAllActiveWorkspaceIds().forEach((id) => destroyWorkspaceStore(id))
  resetWindowPaneStoreForTests()
  useSidebarStore.setState({ repos: [] })
  vi.restoreAllMocks()
})

/**
 * `WorkspaceView`'s very first line is `getOrCreateWorkspaceStore(wsId)` — the
 * store is the value of the `WorkspaceStoreContext` it provides, so it is
 * minted IN THE RENDER PATH, and for a workspace `WorkspaceHost` force-appends
 * (the routed one, a pane's one) no store exists until that render runs.
 * Registering it notified the registry's watchers, and `IDEShell`'s own
 * `useSyncExternalStore` hooks (`useActivePaneWorkspaceId`,
 * `useViewWorkspaceIds`, `usePaneWorkspaceIds`) are exactly those watchers —
 * so React was told to re-render a still-rendering ancestor:
 *
 *   Cannot update a component (`IDEShell`) while rendering a different
 *   component (`WorkspaceView`).
 *
 * The shape below is that path, minimised: an ancestor whose answer comes from
 * the registry-wide subscription, and a child that mints the store for the
 * answer it was just given. The sidebar HINT is what makes it reachable —
 * `resolveChatWorkspaceId` answers from the sidebar tree before any store
 * exists, which is precisely when the child has one to mint.
 */
function WorkspaceSlot({ wsId }: { wsId: string }) {
  getOrCreateWorkspaceStore(wsId)
  return null
}

function Shell() {
  const wsId = useActivePaneWorkspaceId()
  return wsId ? <WorkspaceSlot wsId={wsId} /> : null
}

function renderPhaseUpdates(calls: unknown[][]): string[] {
  return calls
    .filter((call) =>
      call.some(
        (arg) => typeof arg === 'string' && arg.includes('while rendering a different component'),
      ),
    )
    .map((call) => call.join(' '))
}

describe('workspace registry notifications vs. the render path', () => {
  it('minting a store while a registry-subscribed ancestor renders does not setState mid-render', () => {
    const errors = vi.spyOn(console, 'error').mockImplementation(() => {})

    render(<Shell />)

    act(() => {
      // The sidebar tree learns the chat (hint source), and the active pane
      // takes it — together they move the ancestor's answer from null to
      // 'ws-a' with NO store registered for 'ws-a' yet, so the child's own
      // render is what mints it.
      useSidebarStore.setState({ repos: [repoWithChat('c1', 'ws-a')] })
      windowPaneStore.getState().paneActions.openChat('c1')
    })

    expect(renderPhaseUpdates(errors.mock.calls)).toEqual([])
    // ...and the child really did mint it — otherwise this asserts nothing.
    expect(getAllActiveWorkspaceIds()).toContain('ws-a')
  })
})
