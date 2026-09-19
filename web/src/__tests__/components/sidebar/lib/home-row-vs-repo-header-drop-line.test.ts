import { beforeEach, describe, expect, it, vi } from 'vitest'
import { allowedModes } from '@/components/sidebar/lib/sidebar-drop-policy'
import { NO_MODES, REORDER_MODES } from '@/components/tree-dnd/drop-core'
import { getInitialState, useSidebarStore } from '@/lib/store/sidebar'
import { useHomeTreeStore } from '@/lib/store/home-tree'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

/**
 * Regression for "a project-home chat/folder can never reorder past a repo
 * header row": the live per-frame hit test rebuilds its target off the DOM
 * attributes `SIDEBAR_DRAG_ROW_SPEC` declares (`tree-dnd/drop-dom.ts`'s
 * `read()`), which never include `repoIcon` — so the two remaining
 * `target.repoIcon` reads in `sidebar-drop-policy.ts` both resolved
 * `undefined` live and refused every mode, while the hand-built full
 * `SidebarRow` in the sibling `sidebar-drop-policy.test.ts` kept passing.
 * Every target here is built the way `read()` builds one.
 */
const { getHomeWorkspaceId } = vi.hoisted(() => ({ getHomeWorkspaceId: vi.fn() }))
vi.mock('@/features/workspace/lib/home-workspace-resolver', () => ({
  getHomeWorkspaceId,
  getHomeOwningChatId: () => null,
}))

function liveDomBranchTarget(id: string, parentId = ''): SidebarRow {
  return {
    kind: 'branch',
    id,
    parentId,
    expanded: false,
    hasChildren: false,
    inRecents: false,
  } as unknown as SidebarRow
}

function homeChatSubject(id: string, homeWorkspaceId: string): SidebarRow {
  return {
    id,
    kind: 'chat',
    parentId: null,
    order: 0,
    label: id,
    ownsWorktree: false,
    workspaceId: homeWorkspaceId,
    working: false,
    hasView: false,
  }
}

function homeFolderSubject(id: string): SidebarRow {
  return {
    id,
    kind: 'folder',
    parentId: null,
    order: 0,
    label: id,
    ownsWorktree: false,
    workspaceId: '',
    working: false,
    hasView: false,
  }
}

describe('a project-home row against a live DOM-reconstructed repo header target', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getHomeWorkspaceId.mockImplementation((projectId: string) =>
      projectId === 'proj-1' ? 'projhome-1' : projectId === 'proj-2' ? 'projhome-2' : null,
    )
    // A repo header row is addressed by the chat that owns the repo's default
    // workspace (`rows-from-repo.ts`), so the live target id is that chat id.
    useSidebarStore.setState({
      ...getInitialState(),
      repos: [
        {
          id: 'repo-alpha',
          projectId: 'proj-1',
          name: 'repo-alpha',
          avatarLabel: 'A',
          avatarColor: 'bg-indigo-700',
          defaultWorkspaceId: 'alpha-home-ws',
          defaultOwningChatId: 'alpha-owner-chat',
          workspaces: [{ id: 'alpha-locked-ws', branch: 'develop', age: '', status: 'locked' }],
          chats: [
            {
              id: 'alpha-locked-owner',
              repoId: 'repo-alpha',
              title: 'develop',
              order: 0,
              ownsWorktree: true,
              workspaceId: 'alpha-locked-ws',
            },
          ],
        },
        {
          id: 'repo-beta',
          projectId: 'proj-1',
          name: 'repo-beta',
          avatarLabel: 'B',
          avatarColor: 'bg-indigo-700',
          defaultWorkspaceId: 'beta-home-ws',
          defaultOwningChatId: 'beta-owner-chat',
          workspaces: [],
        },
        {
          id: 'repo-gamma',
          projectId: 'proj-2',
          name: 'repo-gamma',
          avatarLabel: 'C',
          avatarColor: 'bg-indigo-700',
          defaultWorkspaceId: 'gamma-home-ws',
          defaultOwningChatId: 'gamma-owner-chat',
          workspaces: [],
        },
      ],
    })
    useHomeTreeStore.setState({
      trees: {
        'proj-1': {
          chats: [
            {
              id: 'homechat-1',
              repoId: '',
              title: 'chat-one',
              order: 0,
              workspaceId: 'projhome-1',
            },
          ],
          folders: [{ id: 'homefolder-1', repoId: '', name: 'homefolder-1', order: 1 }],
        },
      },
    })
  })

  it('offers the reorder modes for a home chat past a repo header row', () => {
    const modes = allowedModes(
      [homeChatSubject('homechat-1', 'projhome-1')],
      liveDomBranchTarget('beta-owner-chat'),
    )
    expect(modes).toEqual(REORDER_MODES)
  })

  it('still refuses a home chat past a repo-internal locked branch row', () => {
    const modes = allowedModes(
      [homeChatSubject('homechat-1', 'projhome-1')],
      liveDomBranchTarget('alpha-locked-owner', 'alpha-owner-chat'),
    )
    expect(modes).toEqual(NO_MODES)
  })

  it("still refuses a home chat past another project's repo header row", () => {
    const modes = allowedModes(
      [homeChatSubject('homechat-1', 'projhome-1')],
      liveDomBranchTarget('gamma-owner-chat'),
    )
    expect(modes).toEqual(NO_MODES)
  })

  it('offers the reorder modes for a home folder past a repo header row', () => {
    const modes = allowedModes(
      [homeFolderSubject('homefolder-1')],
      liveDomBranchTarget('beta-owner-chat'),
    )
    expect(modes).toEqual(REORDER_MODES)
  })

  it("refuses a home folder past another project's repo header row", () => {
    const modes = allowedModes(
      [homeFolderSubject('homefolder-1')],
      liveDomBranchTarget('gamma-owner-chat'),
    )
    expect(modes).toEqual(NO_MODES)
  })

  it('refuses a home folder past a repo-internal branch row', () => {
    const modes = allowedModes(
      [homeFolderSubject('homefolder-1')],
      liveDomBranchTarget('alpha-locked-owner', 'alpha-owner-chat'),
    )
    expect(modes).toEqual(NO_MODES)
  })
})
