import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { SpaceScroller } from '@/components/sidebar/space-scroller'
import { useHomeTreeStore } from '@/lib/store/home-tree'
import {
  usePendingCreatesStore,
  getInitialPendingCreatesState,
  type PendingCreateEntry,
} from '@/lib/store/pending-creates'
import { __resetWorkspaceScopesForTest } from '@/lib/workspace-scope'
import { rowsFromPending } from '@/components/sidebar/lib/rows-from-pending'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import type { RecentsBandEntry } from '@/components/sidebar/recents-band'
import type { Project } from '@/lib/types'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
}))

vi.mock('@/features/workspace/lib/home-workspace-resolver', () => ({
  useHomeWorkspaceState: () => ({ wsId: 'home-ws-1', owningChatId: 'home-owner', error: false }),
  ensureHomeWorkspaceResolved: vi.fn(),
}))

vi.mock('@/components/layout/space-content-actions', () => ({
  handleCreateHomeThread: vi.fn(),
}))

vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: vi.fn(), success: vi.fn(), info: vi.fn() },
}))

vi.mock('@/components/sidebar/lib/row-actions', () => ({
  performCreateHomeFolder: vi.fn(),
}))

vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getAllActiveWorkspaceIds: () => [],
  getWorkspaceStore: () => undefined,
  getOrCreateWorkspaceStore: () => ({
    getState: () => ({
      panes: {},
      agentChats: { working: {}, chats: [] },
      dormantArrangements: [],
    }),
    subscribe: () => () => {},
  }),
  subscribeChatWorking: () => () => {},
  readChatWorking: () => false,
}))

function makeProject(id: string): Project {
  return { id, name: id, path: `/repos/${id}`, lastActivity: new Date('2026-08-28T00:00:00Z') }
}

const noRecents = () => [] as RecentsBandEntry[]

function renderScroller(rowsForProject: (projectId: string) => SidebarRow[] = () => []) {
  return render(
    <SpaceScroller
      projects={[makeProject('p1')]}
      activeProjectId="p1"
      onActiveProjectChange={vi.fn()}
      rowsForProject={rowsForProject}
      recentsForProject={noRecents}
      onOpen={vi.fn()}
      onTrash={vi.fn()}
      onCreate={vi.fn()}
      onFocusRecent={vi.fn()}
      onCloseRecent={vi.fn()}
      onCloseChatRecent={vi.fn()}
      onDrop={vi.fn()}
      onPaneDrop={vi.fn()}
      onTrashProject={vi.fn()}
    />,
  )
}

/**
 * The window between the daemon's own `created`/`placement_set` frames (which
 * fire off MintChat, BEFORE StartRunner / SpawnChatWithOwnWorktree finish) and
 * the POST response that carries the new id. The reseed those frames trigger
 * puts the REAL row in the tree while the pending entry still has no `realId`
 * to hide it by — so the user sees the spinner row AND an "Untitled chat" row
 * for the whole CLI-spawn (thread) or worktree-provision (fork) duration, then
 * the real row vanishes when `attachRealId` lands, then reappears on clear.
 */
describe('a create in flight, before its POST has answered', () => {
  beforeEach(() => {
    HTMLElement.prototype.scrollTo = vi.fn()
    useHomeTreeStore.setState({ trees: {} })
    usePendingCreatesStore.setState(getInitialPendingCreatesState())
    __resetWorkspaceScopesForTest()
  })

  it('draws ONE stand-in for the create, never the pending row beside the not-yet-attached real row', () => {
    useHomeTreeStore.setState({
      trees: {
        p1: {
          chats: [
            {
              id: 'home-owner',
              repoId: '',
              workspaceId: 'home-ws-1',
              title: '',
              order: 0,
              ownsWorktree: true,
            },
            {
              id: 'existing',
              repoId: '',
              workspaceId: 'home-ws-1',
              title: 'Older thread',
              order: 0,
            },
            // Landed via the `created` frame's reseed; the POST is still open.
            { id: 'real-1', repoId: '', workspaceId: 'home-ws-1', title: '', order: 1 },
          ],
          folders: [],
        },
      },
    })
    const entry: PendingCreateEntry = {
      tempId: 'pending-1',
      kind: 'chat',
      projectId: 'p1',
      parentId: '',
      order: 1,
      workspaceId: 'home-ws-1',
      ownsWorktree: false,
      status: 'creating',
      label: '',
      // What the panel held when the "+" was clicked — the only way to tell a
      // row that arrived FOR this create apart from one that was already there.
      rowIdsAtClick: ['existing'],
    }
    usePendingCreatesStore.setState({ entries: [entry] })

    // `SidebarTreeSurface.rowsForProjectFn` merges the pending rows in here.
    renderScroller(() => rowsFromPending(usePendingCreatesStore.getState().entries))

    expect(screen.getByText('Older thread')).toBeInTheDocument()
    expect(screen.getByText('New thread')).toBeInTheDocument()
    // The real row for THIS create must not be drawn until its pending entry clears.
    expect(screen.queryByText(UNTITLED_CHAT_LABEL)).not.toBeInTheDocument()
  })
})
