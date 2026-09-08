import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  SIDEBAR_DROP_POLICY,
  allowedModes,
  edgeBandFor,
} from '@/components/sidebar/lib/sidebar-drop-policy'
import {
  ALL_MODES,
  EDGE_BAND_CONTAINER,
  EDGE_BAND_HEAVY,
  NO_MODES,
  REORDER_MODES,
} from '@/components/tree-dnd/drop-core'
import { getInitialState, useSidebarStore } from '@/lib/store/sidebar'
import {
  destroyWorkspaceStore,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import { useHomeTreeStore } from '@/lib/store/home-tree'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

// `resolveHomeRowScope` (home-tree.ts) reads this to name the project a
// resolved home row belongs to — a real async fetch+cache round trip these
// tests have no reason to exercise.
const { getHomeWorkspaceId } = vi.hoisted(() => ({ getHomeWorkspaceId: vi.fn() }))
vi.mock('@/features/workspace/lib/home-workspace-resolver', () => ({ getHomeWorkspaceId }))

function makeRow(over: Partial<SidebarRow> & { id: string }): SidebarRow {
  return {
    kind: 'branch',
    parentId: null,
    order: 0,
    label: over.id,
    ownsWorktree: true,
    workspaceId: over.id,
    working: false,
    hasView: false,
    ...over,
  }
}

describe('SIDEBAR_DROP_POLICY', () => {
  beforeEach(() => {
    // The workspace-store registry is MODULE state that outlives a test, and
    // the chat cases below seed a live `agentChats.working` map into it — left
    // behind, that would refuse a drag in a later case for a reason that case
    // never set up.
    destroyWorkspaceStore('ws-1')
    vi.clearAllMocks()
    useHomeTreeStore.setState({ trees: {} })
    // repo-1 and repo-2 share proj-1; repo-3 is a different project entirely.
    useSidebarStore.setState({
      ...getInitialState(),
      repos: [
        {
          id: 'repo-1',
          projectId: 'proj-1',
          name: 'repo-1',
          avatarLabel: 'A',
          avatarColor: 'bg-indigo-700',
          defaultWorkspaceId: 'home-1',
          workspaces: [
            { id: 'ws-1', branch: 'feature-a', age: '' },
            // A protected branch — develop — sitting alongside ws-1 under home-1.
            { id: 'ws-locked', branch: 'develop', age: '', status: 'locked' },
          ],
          folders: [{ id: 'folder-1', repoId: 'repo-1', name: 'Bugs', order: 0 }],
        },
        {
          id: 'repo-2',
          projectId: 'proj-1',
          name: 'repo-2',
          avatarLabel: 'B',
          avatarColor: 'bg-indigo-700',
          defaultWorkspaceId: 'home-2',
          workspaces: [{ id: 'ws-2', branch: 'feature-b', age: '' }],
        },
        {
          id: 'repo-3',
          projectId: 'proj-2',
          name: 'repo-3',
          avatarLabel: 'C',
          avatarColor: 'bg-indigo-700',
          defaultWorkspaceId: 'home-3',
          workspaces: [],
        },
      ],
    })
  })

  it('a working row allows no drop mode', () => {
    const subject = makeRow({ id: 'ws-1', working: true })
    const target = makeRow({ id: 'home-1' })
    expect(SIDEBAR_DROP_POLICY.allowedModes([subject], target)).toEqual(NO_MODES)
  })

  it('cross-project drag is refused', () => {
    const subject = makeRow({ id: 'ws-1' }) // repo-1, proj-1
    const target = makeRow({ id: 'home-3' }) // repo-3, proj-2
    expect(SIDEBAR_DROP_POLICY.allowedModes([subject], target)).toEqual(NO_MODES)
  })

  it('cross-repo is legal only for a row owning no worktree', () => {
    const subjectNoWorktree = makeRow({ id: 'ws-1', ownsWorktree: false })
    const subjectWithWorktree = makeRow({ id: 'ws-1', ownsWorktree: true })
    const target = makeRow({ id: 'ws-2' }) // repo-2, same project as ws-1's repo-1
    expect(SIDEBAR_DROP_POLICY.allowedModes([subjectNoWorktree], target)).not.toEqual(NO_MODES)
    expect(SIDEBAR_DROP_POLICY.allowedModes([subjectWithWorktree], target)).toEqual(NO_MODES)
  })

  it('a same-repo drag is allowed in full', () => {
    const subject = makeRow({ id: 'ws-1' })
    const target = makeRow({ id: 'home-1' })
    expect(SIDEBAR_DROP_POLICY.allowedModes([subject], target)).toEqual(ALL_MODES)
  })

  it('refuses to drop a row onto itself', () => {
    const row = makeRow({ id: 'ws-1' })
    expect(SIDEBAR_DROP_POLICY.allowedModes([row], row)).toEqual(NO_MODES)
  })

  // The repo-scoped half of the same bug the home-folder case below catches:
  // `resolveRowRepo` is deliberately chat-blind (a DRAGGED chat is exempt
  // from the same-repo rule), but that exemption has nothing to do with a
  // FOLDER reordering past a plain repo CHAT sibling — the two share one
  // sibling order space same as in a project's home tree.
  it('allows a repo folder to reorder/file relative to a plain CHAT in the same repo', () => {
    useSidebarStore.setState((s) => ({
      repos: s.repos.map((r) =>
        r.id === 'repo-1'
          ? { ...r, chats: [{ id: 'chat-1', repoId: 'repo-1', title: 'testing', order: 1 }] }
          : r,
      ),
    }))
    const folder = makeRow({ id: 'folder-1', kind: 'folder', workspaceId: null })
    const chat = makeRow({ id: 'chat-1', kind: 'chat', workspaceId: null })
    expect(SIDEBAR_DROP_POLICY.allowedModes([folder], chat)).toEqual(ALL_MODES)
  })

  it('refuses a mixed-kind selection', () => {
    const subjects = [
      makeRow({ id: 'ws-1', kind: 'branch' }),
      makeRow({ id: 'folder-1', kind: 'folder', workspaceId: null }),
    ]
    const target = makeRow({ id: 'home-1' })
    expect(SIDEBAR_DROP_POLICY.allowedModes(subjects, target)).toEqual(NO_MODES)
  })

  it('refuses when a subject cannot be resolved against the live store', () => {
    const subject = makeRow({ id: 'ghost-row' })
    const target = makeRow({ id: 'ws-2' })
    expect(SIDEBAR_DROP_POLICY.allowedModes([subject], target)).toEqual(NO_MODES)
  })

  it('refuses when the target cannot be resolved against the live store', () => {
    const subject = makeRow({ id: 'ws-1' })
    const target = makeRow({ id: 'ghost-target' })
    expect(SIDEBAR_DROP_POLICY.allowedModes([subject], target)).toEqual(NO_MODES)
  })

  it('an empty selection has nothing to drop', () => {
    const target = makeRow({ id: 'ws-2' })
    expect(SIDEBAR_DROP_POLICY.allowedModes([], target)).toEqual(NO_MODES)
  })

  it('a locked (protected-branch) row reorders among its own siblings but cannot re-parent', () => {
    // ws-locked and ws-1 both sit under home-1 (same parent).
    const subject = makeRow({ id: 'ws-locked', workspaceId: 'ws-locked', parentId: 'home-1' })
    const sibling = makeRow({ id: 'ws-1', workspaceId: 'ws-1', parentId: 'home-1' })
    // folder-1 lives in the same repo but sits at the repo root, not under home-1.
    const otherContainer = makeRow({
      id: 'folder-1',
      kind: 'folder',
      workspaceId: null,
      parentId: null,
    })

    expect(SIDEBAR_DROP_POLICY.allowedModes([subject], sibling)).toEqual(REORDER_MODES)
    expect(SIDEBAR_DROP_POLICY.allowedModes([subject], otherContainer)).toEqual(NO_MODES)
  })

  it('a locked row never gets "after" on an already-expanded target (that slot re-parents)', () => {
    const subject = makeRow({ id: 'ws-locked', workspaceId: 'ws-locked', parentId: 'home-1' })
    const sibling = makeRow({
      id: 'ws-1',
      workspaceId: 'ws-1',
      parentId: 'home-1',
    }) as SidebarRow & { expanded?: boolean; hasChildren?: boolean }
    const expandedSibling = { ...sibling, expanded: true, hasChildren: true }

    expect(SIDEBAR_DROP_POLICY.allowedModes([subject], expandedSibling)).toEqual({
      before: true,
      after: false,
      into: false,
    })
  })

  it('a folder row resolves through the folders array, not workspaceId', () => {
    const subject = makeRow({ id: 'folder-1', kind: 'folder', workspaceId: null })
    const target = makeRow({ id: 'ws-1' }) // same repo (repo-1)
    expect(SIDEBAR_DROP_POLICY.allowedModes([subject], target)).toEqual(ALL_MODES)
  })

  it('gives a folder target the container band and every other kind the heavy one', () => {
    expect(SIDEBAR_DROP_POLICY.edgeBandFor('folder')).toBe(EDGE_BAND_CONTAINER)
    expect(SIDEBAR_DROP_POLICY.edgeBandFor('branch')).toBe(EDGE_BAND_HEAVY)
    expect(SIDEBAR_DROP_POLICY.edgeBandFor('chat')).toBe(EDGE_BAND_HEAVY)
    expect(SIDEBAR_DROP_POLICY.edgeBandFor('workflow')).toBe(EDGE_BAND_HEAVY)
  })

  it('exports the same functions the policy object wraps', () => {
    expect(SIDEBAR_DROP_POLICY.allowedModes).toBe(allowedModes)
    expect(SIDEBAR_DROP_POLICY.edgeBandFor).toBe(edgeBandFor)
  })

  // The repo-scope walk resolved NOTHING for a chat subject or target and so
  // refused every mode — which, back when every chat row in the app was drawn
  // by RecentsBand, refused every Recents-entry drag there was and left spec
  // §8.1's own target table (middle of a Recents entry / above-below one) with
  // no reachable implementation.
  //
  // `Repo` does hold chats now, but the exemption is unchanged and is NOT an
  // artifact of that: a chat is workspace-scoped for placement, and §8.3 makes
  // cross-repo drag legal precisely for a row that owns no worktree — so
  // resolving one against the same-repo rule would refuse what the spec allows.
  describe('chat rows are workspace-scoped, not repo-scoped', () => {
    const chatRow = (id: string, over: Partial<SidebarRow> = {}) =>
      makeRow({ id, kind: 'chat', ownsWorktree: false, workspaceId: 'ws-1', ...over })

    it('allows a chat drop onto another chat, though neither is in any repo', () => {
      expect(SIDEBAR_DROP_POLICY.allowedModes([chatRow('chat-a')], chatRow('chat-b'))).toEqual(
        ALL_MODES,
      )
    })

    it('allows a cross-workspace chat drop — §8.3 makes it legal for a row with no worktree', () => {
      // The one case with no backend endpoint is answered by planChatDrop's own
      // explicit toast, not by a silent refusal here (§8.3's refusal affordance
      // does not exist yet, so a refusal would explain nothing).
      const subject = chatRow('chat-a', { workspaceId: 'ws-1' })
      const target = chatRow('chat-b', { workspaceId: 'ws-2' })
      expect(SIDEBAR_DROP_POLICY.allowedModes([subject], target)).toEqual(ALL_MODES)
    })

    it('still refuses a WORKING chat — §8.3 outranks the exemption', () => {
      const subject = chatRow('chat-a', { working: true })
      expect(SIDEBAR_DROP_POLICY.allowedModes([subject], chatRow('chat-b'))).toEqual(NO_MODES)
    })

    // A TREE chat row reports `working: false` always — it is built from the
    // repo's reseeded chat list, which has no per-turn push to ride (see
    // `rows-from-repo.ts`, where that is a decision, not an oversight). The
    // refusal above therefore could not see it, and the same chat dragged from
    // the tree instead of from Recents skipped the client-side refusal
    // entirely, learning it only from a rejected round trip as a raw error
    // toast. The live map Recents reads is asked for directly here.
    it('refuses a tree chat row that IS working despite its row saying false', () => {
      const store = getOrCreateWorkspaceStore('ws-1')
      store.setState({
        agentChats: { ...store.getState().agentChats, working: { 'chat-a': true } },
      })
      const subject = chatRow('chat-a', { working: false })
      expect(SIDEBAR_DROP_POLICY.allowedModes([subject], chatRow('chat-b'))).toEqual(NO_MODES)
    })

    it('allows a tree chat row whose live turn state says idle', () => {
      const store = getOrCreateWorkspaceStore('ws-1')
      store.setState({
        agentChats: { ...store.getState().agentChats, working: { 'chat-a': false } },
      })
      expect(
        SIDEBAR_DROP_POLICY.allowedModes(
          [chatRow('chat-a', { working: false })],
          chatRow('chat-b'),
        ),
      ).toEqual(ALL_MODES)
    })

    // A chat whose workspace is not mounted has no live answer to give. It must
    // stay draggable — the server refuses it if it is genuinely mid-turn — not
    // be refused on the absence of information.
    it('allows a chat no mounted workspace knows about', () => {
      expect(
        SIDEBAR_DROP_POLICY.allowedModes(
          [chatRow('chat-unknown', { working: false })],
          chatRow('chat-b'),
        ),
      ).toEqual(ALL_MODES)
    })

    it('still refuses dropping a chat onto itself', () => {
      expect(SIDEBAR_DROP_POLICY.allowedModes([chatRow('chat-a')], chatRow('chat-a'))).toEqual(
        NO_MODES,
      )
    })

    it('refuses a chat onto a branch row — a branch is not one of a chat’s threads', () => {
      // `planChatDrop` refuses a non-chat, non-folder target too; refusing
      // here means the indicator never promises a move that would then
      // quietly do nothing.
      expect(
        SIDEBAR_DROP_POLICY.allowedModes([chatRow('chat-a')], makeRow({ id: 'ws-1' })),
      ).toEqual(NO_MODES)
    })

    // The literal "can't group chats into a folder" gap, caught live: a
    // `kind: 'folder'` row is one aggregate in the current unified row
    // model (rows-from-repo.ts's folder push is the same
    // `AgentChatFolder`/`ChatsFolderWireDTO` `planChatDrop` targets), not
    // the two separate "repo folder" / "chat folder" concepts an earlier,
    // pre-unification version of this policy refused to mix.
    it('allows a chat onto a folder row — filing it in is a real move now', () => {
      expect(
        SIDEBAR_DROP_POLICY.allowedModes(
          [chatRow('chat-a')],
          makeRow({ id: 'folder-1', kind: 'folder', workspaceId: null }),
        ),
      ).toEqual(ALL_MODES)
    })

    it('refuses a tree row onto a chat — a branch is not one of a chat’s threads', () => {
      expect(
        SIDEBAR_DROP_POLICY.allowedModes([makeRow({ id: 'ws-1' })], chatRow('chat-a')),
      ).toEqual(NO_MODES)
    })
  })

  // Project-home folders (rows-from-home.ts) ride no repo at all — the
  // literal "can't drag/group a home folder" gap, caught live: `resolveRowRepo`
  // can never see them, so the repo-scoped walk below refused every one.
  describe('project-home folders are project-scoped, not repo-scoped', () => {
    const homeFolderRow = (id: string, over: Partial<SidebarRow> = {}) =>
      makeRow({ id, kind: 'folder', workspaceId: null, ...over })

    it('allows reordering/filing a home folder relative to another row in the SAME project’s home tree', () => {
      getHomeWorkspaceId.mockReturnValue('home-ws-1')
      useHomeTreeStore.setState({
        trees: {
          'proj-1': {
            chats: [],
            folders: [
              { id: 'home-folder-1', repoId: '', name: 'Notes', order: 0 },
              { id: 'home-folder-2', repoId: '', name: 'Ideas', order: 1 },
            ],
          },
        },
      })

      expect(
        SIDEBAR_DROP_POLICY.allowedModes(
          [homeFolderRow('home-folder-1')],
          homeFolderRow('home-folder-2'),
        ),
      ).toEqual(ALL_MODES)
    })

    it('refuses a home folder dropped onto a DIFFERENT project’s home folder', () => {
      getHomeWorkspaceId.mockImplementation((projectId: string) =>
        projectId === 'proj-2' ? 'home-ws-2' : 'home-ws-1',
      )
      useHomeTreeStore.setState({
        trees: {
          'proj-1': {
            chats: [],
            folders: [{ id: 'home-folder-1', repoId: '', name: 'A', order: 0 }],
          },
          'proj-2': {
            chats: [],
            folders: [{ id: 'home-folder-2', repoId: '', name: 'B', order: 0 }],
          },
        },
      })

      expect(
        SIDEBAR_DROP_POLICY.allowedModes(
          [homeFolderRow('home-folder-1')],
          homeFolderRow('home-folder-2'),
        ),
      ).toEqual(NO_MODES)
    })

    // Caught live: "New folder" (a project-home folder) could reorder past
    // another home FOLDER but never past a home CHAT — the two share one
    // sibling order space, so nothing about the target being a chat rather
    // than a folder should matter. Root cause: the block above this one
    // (`kind === 'chat' || target.kind === 'chat'`) used to catch a FOLDER
    // subject dragged onto a CHAT target too, saw the subject's kind was not
    // 'chat', and refused before this block — the one that actually knows
    // how to resolve a home chat target — was ever reached.
    it('allows filing/reordering a home folder relative to a home CHAT in the SAME project', () => {
      getHomeWorkspaceId.mockReturnValue('home-ws-1')
      useHomeTreeStore.setState({
        trees: {
          'proj-1': {
            chats: [{ id: 'home-chat-1', repoId: '', title: 'testing', order: 0 }],
            folders: [{ id: 'home-folder-1', repoId: '', name: 'Notes', order: 1 }],
          },
        },
      })
      const homeChatRow = makeRow({ id: 'home-chat-1', kind: 'chat', workspaceId: null })

      expect(
        SIDEBAR_DROP_POLICY.allowedModes([homeFolderRow('home-folder-1')], homeChatRow),
      ).toEqual(ALL_MODES)
    })

    it('refuses mixing a home folder with a repo folder in one drag', () => {
      getHomeWorkspaceId.mockReturnValue('home-ws-1')
      useHomeTreeStore.setState({
        trees: {
          'proj-1': {
            chats: [],
            folders: [{ id: 'home-folder-1', repoId: '', name: 'A', order: 0 }],
          },
        },
      })

      expect(
        SIDEBAR_DROP_POLICY.allowedModes(
          [homeFolderRow('home-folder-1'), makeRow({ id: 'folder-1', kind: 'folder' })],
          homeFolderRow('home-folder-1'),
        ),
      ).toEqual(NO_MODES)
    })
  })

  // Caught live: dragging a repo's own header row did nothing at all —
  // `planTreeRowDrop` had no call to construct for it, and this matrix had no
  // branch that recognised it either, so it fell into the generic same-repo
  // walk and was refused outright (`resolveRowRepo` never resolves a repo's
  // own id). Its placement lives on `domain.Repository`, a different
  // aggregate from every ordinary branch/chat/folder row above.
  describe('a repo header row is project-home-scoped, not repo-scoped', () => {
    const repoRow = (id: string, projectId: string, repoId: string, over: Partial<SidebarRow> = {}) =>
      makeRow({
        id,
        kind: 'branch',
        repoIcon: { repoId, projectId, name: repoId, avatarLabel: 'A', avatarColor: 'bg-indigo-700' },
        ...over,
      })

    it('reorders (never nests) relative to another repo header in the SAME project', () => {
      expect(
        SIDEBAR_DROP_POLICY.allowedModes(
          [repoRow('home-1', 'proj-1', 'repo-1')],
          repoRow('home-2', 'proj-1', 'repo-2'),
        ),
      ).toEqual(REORDER_MODES)
    })

    it('refuses a repo header dropped onto a DIFFERENT project’s repo header', () => {
      expect(
        SIDEBAR_DROP_POLICY.allowedModes(
          [repoRow('home-1', 'proj-1', 'repo-1')],
          repoRow('home-3', 'proj-2', 'repo-3'),
        ),
      ).toEqual(NO_MODES)
    })

    it('refuses a repo header dropped onto an ORDINARY branch row (not a repo header)', () => {
      const ordinaryBranch = makeRow({ id: 'ws-1', kind: 'branch', workspaceId: 'ws-1' })
      expect(
        SIDEBAR_DROP_POLICY.allowedModes([repoRow('home-1', 'proj-1', 'repo-1')], ordinaryBranch),
      ).toEqual(NO_MODES)
    })

    it('reorders (never nests) relative to a home CHAT in the same project', () => {
      getHomeWorkspaceId.mockReturnValue('home-ws-1')
      useHomeTreeStore.setState({
        trees: {
          'proj-1': {
            chats: [{ id: 'home-chat-1', repoId: '', title: 'testing', order: 0 }],
            folders: [],
          },
        },
      })
      const homeChatRow = makeRow({ id: 'home-chat-1', kind: 'chat', workspaceId: null })

      expect(
        SIDEBAR_DROP_POLICY.allowedModes([repoRow('home-1', 'proj-1', 'repo-1')], homeChatRow),
      ).toEqual(REORDER_MODES)
    })

    it('reorders OR nests into a home FOLDER in the same project', () => {
      getHomeWorkspaceId.mockReturnValue('home-ws-1')
      useHomeTreeStore.setState({
        trees: {
          'proj-1': {
            chats: [],
            folders: [{ id: 'home-folder-1', repoId: '', name: 'Notes', order: 0 }],
          },
        },
      })
      const homeFolderRow = makeRow({ id: 'home-folder-1', kind: 'folder', workspaceId: null })

      expect(
        SIDEBAR_DROP_POLICY.allowedModes([repoRow('home-1', 'proj-1', 'repo-1')], homeFolderRow),
      ).toEqual(ALL_MODES)
    })

    // Caught live: "New folder" sits nested inside a home CHAT (a legal spot
    // for a folder) — reordering a repo "past" it silently promised a move
    // that would file the repo under that CHAT, refused server-side with a
    // raw 400 the drag indicator never should have offered in the first
    // place. Nesting INTO that same folder is still fine — it is the
    // container the FOLDER itself sits in that disqualifies a reorder, not
    // the folder as a destination.
    it('refuses to reorder past a home folder that is itself nested inside a CHAT — but still allows nesting INTO it', () => {
      getHomeWorkspaceId.mockReturnValue('home-ws-1')
      useHomeTreeStore.setState({
        trees: {
          'proj-1': {
            chats: [{ id: 'home-chat-1', repoId: '', title: 'testing', order: 0 }],
            folders: [{ id: 'home-folder-1', repoId: '', name: 'Notes', parentId: 'home-chat-1', order: 0 }],
          },
        },
      })
      const nestedFolderRow = makeRow({
        id: 'home-folder-1',
        kind: 'folder',
        workspaceId: null,
        parentId: 'home-chat-1',
      })

      expect(
        SIDEBAR_DROP_POLICY.allowedModes([repoRow('home-1', 'proj-1', 'repo-1')], nestedFolderRow),
      ).toEqual({ before: false, after: false, into: true })
    })

    it('refuses a repo header dropped onto a DIFFERENT project’s home folder', () => {
      getHomeWorkspaceId.mockImplementation((projectId: string) =>
        projectId === 'proj-2' ? 'home-ws-2' : 'home-ws-1',
      )
      useHomeTreeStore.setState({
        trees: {
          'proj-2': {
            chats: [],
            folders: [{ id: 'home-folder-2', repoId: '', name: 'Ideas', order: 0 }],
          },
        },
      })
      const homeFolderRow = makeRow({ id: 'home-folder-2', kind: 'folder', workspaceId: null })

      expect(
        SIDEBAR_DROP_POLICY.allowedModes([repoRow('home-1', 'proj-1', 'repo-1')], homeFolderRow),
      ).toEqual(NO_MODES)
    })
  })
})
