import { describe, expect, it, vi, afterEach, beforeEach } from 'vitest'

// Mocked so the real registry's store creation doesn't need a real
// IndexedDB/localStorage write path — same setup recents-actions.test.ts
// uses for exercising the real registry.
vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/editor/stores/buffer-session-persistence', () => ({
  saveSessionToStore: vi.fn(),
  clearQueuedWorkspaceSessionSave: vi.fn(),
}))

import { performSidebarPaneDrop } from '@/components/sidebar/lib/drop-actions'
import {
  getOrCreateWorkspaceStore,
  destroyWorkspaceStore,
  getAllActiveWorkspaceIds,
  setActiveWorkspaceId,
} from '@/features/workspace/stores/workspace-store-registry'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

/**
 * `performSidebarPaneDrop` — spec §8.1's other two targets (middle of a pane,
 * edge of a pane) and §8.2's rules for what a drop onto one of them means.
 *
 * "middle of a Recents entry" / "above·below a Recents entry" (the other two
 * targets in §8.1's table) are NOT tested here — they resolve through
 * `onDrop`/`performSidebarDrop`, a disclosed placeholder this task
 * deliberately leaves alone (see the comment on `performSidebarDrop` in
 * drop-actions.ts).
 */

const chatRow = (id: string, wsId = 'ws-1', over: Partial<SidebarRow> = {}): SidebarRow => ({
  id,
  kind: 'chat',
  parentId: null,
  order: 0,
  label: id,
  ownsWorktree: false,
  workspaceId: wsId,
  working: false,
  hasView: false,
  ...over,
})

// `openChatIntoPane` refuses unless the dragged chat's own workspace IS the
// active one (Fix round 1 — see the "cross-workspace safety" describe block
// below) — every test in this file drags a chat that belongs where it drops,
// so `ws-1` is the active workspace throughout unless a test says otherwise.
beforeEach(() => {
  setActiveWorkspaceId('ws-1')
})

afterEach(() => {
  getAllActiveWorkspaceIds().forEach((id) => destroyWorkspaceStore(id))
})

describe('performSidebarPaneDrop — non-chat subjects', () => {
  it('ignores branch/folder/workflow rows — no pane has an "open into" meaning for them yet', () => {
    const store = getOrCreateWorkspaceStore('ws-1')
    const before = store.getState().panes[ROOT_PANE_ID]

    performSidebarPaneDrop(
      [
        chatRow('branch-a', 'ws-1', { kind: 'branch' }),
        chatRow('folder-a', 'ws-1', { kind: 'folder' }),
        chatRow('flow-a', 'ws-1', { kind: 'workflow' }),
      ],
      ROOT_PANE_ID,
      'center',
    )

    expect(store.getState().panes[ROOT_PANE_ID]).toEqual(before)
  })

  it('is a no-op for a chat row with no owning workspace', () => {
    expect(() =>
      performSidebarPaneDrop(
        [chatRow('c1', 'ws-1', { workspaceId: null })],
        ROOT_PANE_ID,
        'center',
      ),
    ).not.toThrow()
  })
})

describe('performSidebarPaneDrop — plain open (spec §8.1 "middle of a pane")', () => {
  it('opens a chat that is not up anywhere into an empty pane, and focuses it', () => {
    const store = getOrCreateWorkspaceStore('ws-1')

    performSidebarPaneDrop([chatRow('c1')], ROOT_PANE_ID, 'center')

    expect(store.getState().panes[ROOT_PANE_ID]?.chatId).toBe('c1')
    expect(store.getState().activePaneId).toBe(ROOT_PANE_ID)
  })
})

describe('performSidebarPaneDrop — already up (spec §8.2)', () => {
  it('a chat already live in another pane goes TO it — reveal, never a second setPaneChat', () => {
    const store = getOrCreateWorkspaceStore('ws-1')
    const otherPane = store.getState().paneActions.splitPane(ROOT_PANE_ID, 'horizontal')!
    store.getState().paneActions.setPaneChat(otherPane, 'c1', 'runner-1')
    store.getState().paneActions.setActivePane(ROOT_PANE_ID)

    performSidebarPaneDrop([chatRow('c1')], ROOT_PANE_ID, 'center')

    // Never opened twice: ROOT_PANE_ID is untouched, and the reveal just
    // refocuses the pane that already has it.
    expect(store.getState().panes[ROOT_PANE_ID]?.chatId).toBeNull()
    expect(store.getState().panes[otherPane]?.chatId).toBe('c1')
    expect(store.getState().activePaneId).toBe(otherPane)
  })

  it('dropping a chat onto the exact pane already showing it is a harmless no-op', () => {
    const store = getOrCreateWorkspaceStore('ws-1')
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'c1', 'runner-1')

    performSidebarPaneDrop([chatRow('c1')], ROOT_PANE_ID, 'right')

    expect(store.getState().panes[ROOT_PANE_ID]?.chatId).toBe('c1')
    expect(Object.keys(store.getState().panes)).toHaveLength(2) // root + bottom only — no split made
  })
})

describe('performSidebarPaneDrop — merging (spec §8.1 "edge of a pane", §8.2)', () => {
  it('an edge drop onto an EMPTY pane still splits, with nothing to merge into', () => {
    const store = getOrCreateWorkspaceStore('ws-1')

    performSidebarPaneDrop([chatRow('c1')], ROOT_PANE_ID, 'right')

    const panes = store.getState().panes
    const newPane = Object.values(panes).find((p) => p.chatId === 'c1')
    expect(newPane).toBeDefined()
    expect(newPane?.id).not.toBe(ROOT_PANE_ID)
    expect(store.getState().dormantArrangements).toEqual([])
  })

  it('a center drop onto an OCCUPIED pane never swaps — it merges instead (rule 1: every drop adds)', () => {
    const store = getOrCreateWorkspaceStore('ws-1')
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'c1', 'runner-1')

    performSidebarPaneDrop([chatRow('c2')], ROOT_PANE_ID, 'center')

    // c1 is still exactly where it was — nothing was evicted.
    expect(store.getState().panes[ROOT_PANE_ID]?.chatId).toBe('c1')
    const newPane = Object.values(store.getState().panes).find((p) => p.chatId === 'c2')
    expect(newPane).toBeDefined()
  })

  it('an edge drop onto an occupied pane groups both chats into one Recents entry ("side by side")', () => {
    const store = getOrCreateWorkspaceStore('ws-1')
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'c1', 'runner-1')

    performSidebarPaneDrop([chatRow('c2')], ROOT_PANE_ID, 'right')

    expect(store.getState().dormantArrangements).toHaveLength(1)
    const [entry] = store.getState().dormantArrangements
    expect([...entry.chatIds].sort()).toEqual(['c1', 'c2'])
  })

  it('merging into a pane already part of an arrangement GROWS it rather than nesting a second one', () => {
    const store = getOrCreateWorkspaceStore('ws-1')
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'c1', 'runner-1')
    performSidebarPaneDrop([chatRow('c2')], ROOT_PANE_ID, 'right') // c1+c2 now grouped

    performSidebarPaneDrop([chatRow('c3')], ROOT_PANE_ID, 'bottom')

    expect(store.getState().dormantArrangements).toHaveLength(1)
    const [entry] = store.getState().dormantArrangements
    expect([...entry.chatIds].sort()).toEqual(['c1', 'c2', 'c3'])
  })

  it('dropping an already-grouped chat elsewhere reveals it in place — the group is untouched', () => {
    // §8.2's "already up → goes to it, never opens twice" is checked BEFORE
    // any merge/zone logic, exactly as §8.4 already establishes for a click
    // ("gone to, never opened twice") — so a chat that is part of a live set
    // cannot be silently re-homed by a plain drop of its own row; only a
    // fresh chat reaching a NEW pane sheds/regroups membership (see
    // pane-slice.test.ts's `setPaneChat`/`groupIntoArrangement` coverage for
    // the "pull one out, survivors kept as a set" bookkeeping itself).
    const store = getOrCreateWorkspaceStore('ws-1')
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'c1', 'runner-1')
    performSidebarPaneDrop([chatRow('c2')], ROOT_PANE_ID, 'right') // c1+c2 now grouped
    const grouped = store.getState().dormantArrangements

    const freshPane = store.getState().paneActions.splitPane(ROOT_PANE_ID, 'vertical')!
    performSidebarPaneDrop([chatRow('c1')], freshPane, 'center')

    expect(store.getState().panes[freshPane]?.chatId).toBeNull()
    expect(store.getState().activePaneId).toBe(ROOT_PANE_ID)
    expect(store.getState().dormantArrangements).toEqual(grouped)
  })
})

// Fix round 1 (real, reviewer-verified regression): RecentsBand is per
// PROJECT, not per workspace (spec §4: "Recents is per space") — a row from
// a workspace that is not currently on screen can still be visible and
// draggable while a DIFFERENT workspace's panes are what you're looking at.
// `paneId` only ever comes from hit-testing the DOM, which only renders the
// ACTIVE workspace's panes (every other retained workspace sits hidden). The
// original implementation resolved `panes[paneId]` against the DRAGGED
// CHAT's own workspace store instead of the pane's real (active) one —
// `ROOT_PANE_ID`/`BOTTOM_PANE_ID` are literal constants every workspace
// store shares (recents-for-project.ts documents the same collision), so
// that lookup does not fail, it silently mutates the WRONG, off-screen
// workspace while the visible one shows nothing happening at all.
describe('performSidebarPaneDrop — cross-workspace safety (Fix round 1)', () => {
  it('refuses a drop when the dragged chat belongs to a workspace other than the one whose pane was actually hit', () => {
    const visible = getOrCreateWorkspaceStore('ws-visible')
    const offscreen = getOrCreateWorkspaceStore('ws-offscreen')
    setActiveWorkspaceId('ws-visible') // ws-visible is what's on screen
    visible.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'already-here', 'runner-1')
    const visibleBefore = visible.getState()
    const offscreenBefore = offscreen.getState()

    // A Recents row for a chat that lives in the OFF-SCREEN workspace,
    // dropped onto the pane the VISIBLE workspace is actually showing.
    performSidebarPaneDrop([chatRow('c1', 'ws-offscreen')], ROOT_PANE_ID, 'right')

    // Neither store moved — not the visible one (never wrote what it wasn't
    // asked to), and CRITICALLY not the off-screen one either (the original
    // bug: it silently split/arranged there, invisible to the user).
    expect(visible.getState().panes).toEqual(visibleBefore.panes)
    expect(visible.getState().rootLayout).toEqual(visibleBefore.rootLayout)
    expect(offscreen.getState().panes).toEqual(offscreenBefore.panes)
    expect(offscreen.getState().rootLayout).toEqual(offscreenBefore.rootLayout)
    expect(offscreen.getState().dormantArrangements).toEqual([])
  })

  it('still works normally once the chat and the active workspace agree', () => {
    const store = getOrCreateWorkspaceStore('ws-visible')
    setActiveWorkspaceId('ws-visible')

    performSidebarPaneDrop([chatRow('c1', 'ws-visible')], ROOT_PANE_ID, 'center')

    expect(store.getState().panes[ROOT_PANE_ID]?.chatId).toBe('c1')
  })
})
