import { beforeEach, describe, expect, it, vi } from 'vitest'
import { allowedModes } from '@/components/sidebar/lib/sidebar-drop-policy'
import { ALL_MODES, NO_MODES, REORDER_MODES } from '@/components/tree-dnd/drop-core'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { seedHomeTree, seedRepos } from '@/__tests__/__fixtures__/sidebar-entities'

// A project-home chat/folder must reorder past a repo header row as a live drag sees it: targets
// are rebuilt from SIDEBAR_DRAG_ROW_SPEC's DOM attributes, which never include `repoIcon`, so
// every target here is built the way drop-dom's `read()` builds one.
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
    // workspace (`deriveRows`), so the live target id is that chat id.
    seedRepos([
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
    ])
    seedHomeTree('proj-1', {
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
    })
  })

  // A chat may land anywhere: beside the header is project home, inside it the repo checkout.
  it('offers every mode for an idle home chat past a repo header row', () => {
    const modes = allowedModes(
      [homeChatSubject('homechat-1', 'projhome-1')],
      liveDomBranchTarget('beta-owner-chat'),
    )
    expect(modes).toEqual(ALL_MODES)
  })

  it('offers every mode past a repo-internal locked branch row and another project’s header', () => {
    const subject = homeChatSubject('homechat-1', 'projhome-1')
    expect(
      allowedModes([subject], liveDomBranchTarget('alpha-locked-owner', 'alpha-owner-chat')),
    ).toEqual(ALL_MODES)
    expect(allowedModes([subject], liveDomBranchTarget('gamma-owner-chat'))).toEqual(ALL_MODES)
  })

  // Working, it may only leave project home: its move waits for the turn to end.
  it('lets a working home chat only into a repo header, never beside it', () => {
    const working = { ...homeChatSubject('homechat-1', 'projhome-1'), working: true }
    expect(allowedModes([working], liveDomBranchTarget('beta-owner-chat'))).toEqual({
      before: false,
      after: false,
      into: true,
    })
    expect(
      allowedModes([working], liveDomBranchTarget('alpha-locked-owner', 'alpha-owner-chat')),
    ).toEqual(ALL_MODES)
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
