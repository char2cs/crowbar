import { act, cleanup, render } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))

import { useActivePaneWorkspaceId } from '@/features/panes/hooks/use-chat-workspace-id'
import {
  destroyWorkspaceStore,
  getAllActiveWorkspaceIds,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { resetWindowPaneStoreForTests } from '@/__tests__/__fixtures__/window-pane-store'
import type { Repo } from '@/core/entities/repo-types'
import { seedRepos } from '@/__tests__/__fixtures__/sidebar-entities'

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
  seedRepos([])
  vi.restoreAllMocks()
})

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
      seedRepos([repoWithChat('c1', 'ws-a')])
      windowPaneStore.getState().paneActions.openChat('c1', { workspaceId: 'ws-a' })
    })

    expect(renderPhaseUpdates(errors.mock.calls)).toEqual([])
    expect(getAllActiveWorkspaceIds()).toContain('ws-a')
  })
})
