import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/lib/persistence/sidebar-ui', () => ({
  saveSidebarUI: vi.fn().mockResolvedValue(undefined),
  loadSidebarUI: vi.fn().mockResolvedValue(null),
}))

import { handleTrash } from '@/components/layout/trash-actions'
import {
  applyPendingRemovals,
  attachRemovalState,
  renderedHiddenIds,
} from '@/components/layout/removal-plan'
import { repoRows } from '@/__tests__/__fixtures__/tree-rows'
import type { Repo } from '@/core/entities/repo-types'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import { seedProjects, seedRepos } from '@/__tests__/__fixtures__/sidebar-entities'
import { readSelect, selectRepos } from '@/core/entities/select'

// The eight-second removal window end to end (handleTrash -> hold -> applyPendingRemovals ->
// deriveRows), because the invariant lives BETWEEN them: a held workspace row stays the branch
// row (both halves hidden) and never resurfaces as a chat bubble.

const PROJECT = {
  id: 'p1',
  name: 'Space',
  path: '/tmp/p1',
  lastActivity: new Date(0),
  order: 0,
  hostId: 'local',
}

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

/** The rows on screen right now: descendants strip out, while the held row's own primary id
 *  stays so it has a row left to transform in place (`renderedHiddenIds`). */
function rowsOnScreen() {
  const { removals: entries, hiddenIds } = treeUiStore.getState()
  const repos = applyPendingRemovals(readSelect(selectRepos), renderedHiddenIds(hiddenIds, entries))
  return attachRemovalState(
    repos.flatMap((r) => repoRows(r)),
    entries,
  )
}

beforeEach(() => {
  treeUiStore.setState(treeUiStore.getInitialState(), true)
  seedRepos([repo()])
  seedProjects([PROJECT])
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
    // The row stays, transformed in place, as the BRANCH row it was, never a chat bubble.
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
    const { removals: entries, cancel } = treeUiStore.getState()
    cancel(entries[0].entryId)

    expect(rowsOnScreen().find((r) => r.id === 'chat-fork')).toMatchObject({
      kind: 'branch',
      ownsWorktree: true,
      workspaceId: 'ws-fork',
    })
  })

  it('takes the owning chat’s id into hiddenIds, not just the workspace’s', () => {
    handleTrash('chat-fork')

    const hidden = treeUiStore.getState().hiddenIds
    expect(hidden.has('ws-fork')).toBe(true)
    expect(hidden.has('chat-fork')).toBe(true)
  })
})
