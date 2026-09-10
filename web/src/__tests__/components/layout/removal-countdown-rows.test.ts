import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/lib/persistence/sidebar-ui', () => ({
  saveSidebarUI: vi.fn().mockResolvedValue(undefined),
  loadSidebarUI: vi.fn().mockResolvedValue(null),
}))

import { handleTrash } from '@/components/layout/space-content-actions'
import { applyPendingRemovals, attachRemovalState, descendantHiddenIds } from '@/components/layout/removal-plan'
import { rowsFromRepo } from '@/components/sidebar/lib/rows-from-repo'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { useRemovalTrayStore, getInitialRemovalState } from '@/lib/store/sidebar-removal'
import { useProjectDataStore } from '@/lib/store/projects'

/**
 * THE EIGHT-SECOND WINDOW, END TO END.
 *
 * Deleting a row in Crowbar is not a delete for the first eight seconds: the
 * rows are HIDDEN and the removal sits in a tray, cancellable. Every one of the
 * three surfaces that window touches is exercised here in the order the app
 * runs them, because the bug this file exists for lived BETWEEN them and no
 * unit test of any one of them could see it:
 *
 *     handleTrash            -> resolve the clicked row, plan what goes
 *     useRemovalTrayStore    -> hold it, hiding those ids
 *     applyPendingRemovals   -> the repo as the sidebar now reads it
 *     rowsFromRepo           -> the rows actually drawn during the countdown
 *
 * What went wrong: a workspace row IS its owning chat (they render as one row),
 * but the hold hid only the `Workspace` half. The chat survived, `rowsFromRepo`
 * found it with no workspace to fold onto, and drew it as a `kind: 'chat'`
 * BUBBLE. So the row the user had just deleted did not disappear — it visibly
 * turned into a conversation and sat there for eight seconds. Testing
 * `foldWorkspaceOwners` alone would have passed the whole time: its inputs were
 * fine, and the skew was introduced one layer up.
 */

const PROJECT = { id: 'p1', name: 'Space', path: '/tmp/p1', lastActivity: '', order: 0 }

/** A repo whose fork rows are unified: one row per (workspace, owning chat). */
function repo(): Repo {
  return {
    id: 'r1',
    projectId: 'p1',
    name: 'checkout',
    avatarLabel: 'C',
    avatarColor: 'avatar-amber',
    order: 0,
    defaultWorkspaceId: 'ws-home',
    defaultBranch: 'main',
    workspaces: [
      {
        id: 'ws-fork',
        branch: 'feature/one',
        age: '',
        order: 0,
        status: 'new',
        owningChatId: 'chat-fork',
      },
      {
        id: 'ws-other',
        branch: 'feature/two',
        age: '',
        order: 1,
        status: 'new',
        owningChatId: 'chat-other',
      },
    ],
    chats: [
      {
        id: 'chat-home',
        repoId: 'r1',
        ownsWorktree: true,
        workspaceId: 'ws-home',
        title: '',
        order: 0,
      },
      {
        id: 'chat-fork',
        repoId: 'r1',
        type: 'chat',
        workspaceId: 'ws-fork',
        ownsWorktree: true,
        title: 'One',
        order: 0,
      },
      {
        id: 'chat-other',
        repoId: 'r1',
        type: 'chat',
        workspaceId: 'ws-other',
        ownsWorktree: true,
        title: 'Two',
        order: 1,
      },
    ],
  }
}

/** The rows on screen right now, exactly as `SidebarTreeSurface` derives them:
 *  descendants strip out, the held row's own primary id stays so it has a
 *  row left to transform in place (`removal-plan.ts`'s `descendantHiddenIds`
 *  / `attachRemovalState`), never the store's raw `hiddenIds` (which still
 *  lists the primary too — that field backs this file's own hiddenIds
 *  assertions, not what the tree actually draws). */
function rowsOnScreen() {
  const entries = useRemovalTrayStore.getState().entries
  const repos = applyPendingRemovals(useSidebarStore.getState().repos, descendantHiddenIds(entries))
  return attachRemovalState(repos.flatMap(rowsFromRepo), entries)
}

beforeEach(() => {
  useRemovalTrayStore.setState(getInitialRemovalState())
  useSidebarStore.setState({ repos: [repo()] })
  useProjectDataStore.setState({ data: { status: 'success', data: [PROJECT] } } as never)
})

describe('a held workspace row during its countdown', () => {
  it('is drawn as ONE branch row before the trash, id’d by its owning chat', () => {
    const row = rowsOnScreen().find((r) => r.id === 'chat-fork')
    expect(row).toMatchObject({ kind: 'branch', ownsWorktree: true, workspaceId: 'ws-fork' })
    // And there is no second row for the same thing — the whole point of the
    // unified row model.
    expect(rowsOnScreen().filter((r) => r.workspaceId === 'ws-fork')).toHaveLength(1)
  })

  it('stays on screen, transformed into its countdown state — it does not turn into a chat bubble', () => {
    expect(handleTrash('chat-fork')).toBe(true)

    const rows = rowsOnScreen()
    const row = rows.find((r) => r.id === 'chat-fork')
    // The row itself stays, transformed in place (explicit product request)
    // — but the regression this file is named for, stated the way the user
    // reported it, is "the branch row transformed into a conversation one":
    // it must stay the BRANCH row it always was, never a chat bubble.
    expect(row).toMatchObject({ kind: 'branch', workspaceId: 'ws-fork' })
    expect(row?.removal?.deadlineAt).not.toBeNull()
    expect(rows.some((r) => r.kind === 'chat' && r.label === 'One')).toBe(false)
  })

  it('leaves every other row exactly as it was', () => {
    handleTrash('chat-fork')

    const other = rowsOnScreen().find((r) => r.id === 'chat-other')
    expect(other).toMatchObject({ kind: 'branch', ownsWorktree: true, workspaceId: 'ws-other' })
  })

  it('puts the row back as a BRANCH row on cancel, not as a bubble', () => {
    handleTrash('chat-fork')
    const { entries, cancel } = useRemovalTrayStore.getState()
    cancel(entries[0].entryId)

    expect(rowsOnScreen().find((r) => r.id === 'chat-fork')).toMatchObject({
      kind: 'branch',
      ownsWorktree: true,
      workspaceId: 'ws-fork',
    })
  })

  it('takes the owning chat’s id into hiddenIds, not just the workspace’s', () => {
    handleTrash('chat-fork')

    const hidden = useRemovalTrayStore.getState().hiddenIds
    expect(hidden.has('ws-fork')).toBe(true)
    expect(hidden.has('chat-fork')).toBe(true)
  })
})

describe('a workspace row whose Workspace record is missing', () => {
  /**
   * The same skew from the other direction, and the reason the fix could not
   * just be "hide the chat too".
   *
   * A freshly forked workspace arrives as a CHAT first — `crowbar_chats` and
   * `crowbar_workspaces` are different streams — so for a beat there is an
   * owning chat with no `Workspace` record anywhere. That row is still a
   * workspace and must still draw as one; drawing a bubble is how a brand-new
   * fork appeared as a conversation until its `WorkspaceDTO` caught up.
   */
  it('still draws a branch row off the chat alone', () => {
    const half = repo()
    half.workspaces = half.workspaces.filter((w) => w.id !== 'ws-fork')
    useSidebarStore.setState({ repos: [half] })

    const row = rowsOnScreen().find((r) => r.id === 'chat-fork')
    expect(row).toMatchObject({ kind: 'branch', ownsWorktree: true, workspaceId: 'ws-fork' })
    // The decoration that genuinely lives on the absent half is absent, rather
    // than faked: a row claiming `locked: false` would offer verbs the daemon
    // then refuses.
    expect(row?.branchName).toBeUndefined()
    expect(row?.locked).toBeUndefined()
  })

  it('is still a real bubble when the chat owns nothing', () => {
    const withBubble = repo()
    withBubble.chats = [
      ...(withBubble.chats ?? []),
      {
        id: 'chat-bubble',
        repoId: 'r1',
        type: 'chat',
        workspaceId: 'ws-fork',
        parentId: 'chat-fork',
        title: 'A thread',
        order: 0,
      },
    ]
    useSidebarStore.setState({ repos: [withBubble] })

    // A thread carries its parent's workspaceId, which is exactly why
    // `workspaceId` alone can never stand in for ownership.
    expect(rowsOnScreen().find((r) => r.id === 'chat-bubble')).toMatchObject({
      kind: 'chat',
      ownsWorktree: false,
    })
  })
})
