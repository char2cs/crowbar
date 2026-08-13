/**
 * Contract pin for every set the sidebar folds away: rows are OPEN by default,
 * and folding one is an explicit, persisted act.
 *
 * Four sets share one record and one writer — repos, workspaces, projects, and
 * the Chats panel's rows.
 *
 * The polarity is the whole test. Showing every project at once is the feature —
 * a fresh install must render the sidebar the mock shows, not a column of closed
 * rows — so "unknown project" has to mean "open". Collapse is how the cost is
 * bought back (a folded project's repo + workspace streams are torn down, see
 * project-visibility.ts), not the resting state that avoids paying it. An empty
 * set that meant "nothing subscribed" would invert the product decision without
 * anything failing.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'

const { saveSidebarUI } = vi.hoisted(() => ({
  saveSidebarUI: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/lib/persistence/sidebar-ui', () => ({
  saveSidebarUI: (...args: unknown[]) => saveSidebarUI(...args),
  loadSidebarUI: vi.fn().mockResolvedValue(null),
}))

import { useSidebarStore } from '@/lib/store/sidebar'

beforeEach(() => {
  vi.clearAllMocks()
  useSidebarStore.setState({
    collapsedRepos: new Set<string>(),
    collapsedWorkspaces: new Set<string>(),
    collapsedProjects: new Set<string>(),
    collapsedChatRows: new Set<string>(),
  })
})

describe('collapsedProjects', () => {
  it('starts empty — every project is OPEN by default', () => {
    expect(useSidebarStore.getState().collapsedProjects.size).toBe(0)
  })

  it('shares its polarity with every other collapse set', () => {
    // All four sets say the same thing: membership means folded away.
    const initial = useSidebarStore.getState()
    expect(initial.collapsedRepos.size).toBe(0)
    expect(initial.collapsedWorkspaces.size).toBe(0)
    expect(initial.collapsedProjects.size).toBe(0)
    expect(initial.collapsedChatRows.size).toBe(0)
  })

  it('toggleProject collapses, then re-opens', () => {
    useSidebarStore.getState().toggleProject('p2')
    expect(useSidebarStore.getState().collapsedProjects.has('p2')).toBe(true)
    useSidebarStore.getState().toggleProject('p2')
    expect(useSidebarStore.getState().collapsedProjects.has('p2')).toBe(false)
  })

  it('toggleProject leaves other projects alone', () => {
    useSidebarStore.getState().toggleProject('p2')
    useSidebarStore.getState().toggleProject('p3')
    useSidebarStore.getState().toggleProject('p2')
    expect([...useSidebarStore.getState().collapsedProjects]).toEqual(['p3'])
  })

  it('hands out a NEW Set so subscribers see the change', () => {
    const before = useSidebarStore.getState().collapsedProjects
    useSidebarStore.getState().toggleProject('p2')
    expect(useSidebarStore.getState().collapsedProjects).not.toBe(before)
  })

  it('persists collapsed projects alongside every other collapse set', () => {
    useSidebarStore.getState().toggleProject('p2')
    expect(saveSidebarUI).toHaveBeenCalledWith({
      collapsedRepos: [],
      collapsedWorkspaces: [],
      collapsedProjects: ['p2'],
      collapsedChatRows: [],
    })
  })

  // The record is written whole by ONE writer, so no toggle can persist its own
  // list over a record whose other three it forgot to carry.
  it('every toggle carries the other three sets through', () => {
    useSidebarStore.getState().toggleProject('p2')
    saveSidebarUI.mockClear()

    useSidebarStore.getState().toggleRepo('r1')
    expect(saveSidebarUI).toHaveBeenLastCalledWith({
      collapsedRepos: ['r1'],
      collapsedWorkspaces: [],
      collapsedProjects: ['p2'],
      collapsedChatRows: [],
    })

    useSidebarStore.getState().toggleWorkspace('w1')
    expect(saveSidebarUI).toHaveBeenLastCalledWith({
      collapsedRepos: ['r1'],
      collapsedWorkspaces: ['w1'],
      collapsedProjects: ['p2'],
      collapsedChatRows: [],
    })

    useSidebarStore.getState().toggleChatRow('f1')
    expect(saveSidebarUI).toHaveBeenLastCalledWith({
      collapsedRepos: ['r1'],
      collapsedWorkspaces: ['w1'],
      collapsedProjects: ['p2'],
      collapsedChatRows: ['f1'],
    })
  })
})

/**
 * The Chats panel's folds, which live here rather than in the panel.
 *
 * The panel is keyed by workspace id, so a workspace switch remounts it — that
 * is deliberate, and it is what a drag, a rename and a selection are supposed to
 * die with. A fold is not: every folded folder sprang open on the way back to a
 * workspace, which is the bug this set exists to fix.
 *
 * One flat set for both row kinds and every workspace: folder ids and chat ids
 * are daemon-minted uuids, so nothing here needs a workspace key, and an id that
 * outlives its row simply matches nothing.
 */
describe('collapsedChatRows', () => {
  it('starts empty — every chat row is OPEN by default', () => {
    expect(useSidebarStore.getState().collapsedChatRows.size).toBe(0)
  })

  it('toggleChatRow folds, then opens again', () => {
    useSidebarStore.getState().toggleChatRow('f1')
    expect(useSidebarStore.getState().collapsedChatRows.has('f1')).toBe(true)
    useSidebarStore.getState().toggleChatRow('f1')
    expect(useSidebarStore.getState().collapsedChatRows.has('f1')).toBe(false)
  })

  it('holds folder ids and chat ids in the one set', () => {
    useSidebarStore.getState().toggleChatRow('f1')
    useSidebarStore.getState().toggleChatRow('c9')
    expect([...useSidebarStore.getState().collapsedChatRows]).toEqual(['f1', 'c9'])
  })

  it('toggleChatRow leaves other rows alone', () => {
    useSidebarStore.getState().toggleChatRow('f1')
    useSidebarStore.getState().toggleChatRow('f2')
    useSidebarStore.getState().toggleChatRow('f1')
    expect([...useSidebarStore.getState().collapsedChatRows]).toEqual(['f2'])
  })

  it('hands out a NEW Set so subscribers see the change', () => {
    const before = useSidebarStore.getState().collapsedChatRows
    useSidebarStore.getState().toggleChatRow('f1')
    expect(useSidebarStore.getState().collapsedChatRows).not.toBe(before)
  })

  it('persists the fold', () => {
    useSidebarStore.getState().toggleChatRow('f1')
    expect(saveSidebarUI).toHaveBeenLastCalledWith({
      collapsedRepos: [],
      collapsedWorkspaces: [],
      collapsedProjects: [],
      collapsedChatRows: ['f1'],
    })
  })

  // "+ in here" files something INSIDE the row, so it opens it. A toggle there
  // would close the row the user is filing into.
  it('openChatRow opens a folded row', () => {
    useSidebarStore.getState().toggleChatRow('f1')
    useSidebarStore.getState().openChatRow('f1')
    expect(useSidebarStore.getState().collapsedChatRows.has('f1')).toBe(false)
  })

  it('openChatRow leaves an already-open row open — it is not a toggle', () => {
    useSidebarStore.getState().openChatRow('f1')
    expect(useSidebarStore.getState().collapsedChatRows.has('f1')).toBe(false)
  })

  it('openChatRow on an already-open row changes nothing and writes nothing', () => {
    const before = useSidebarStore.getState().collapsedChatRows
    useSidebarStore.getState().openChatRow('f1')
    expect(useSidebarStore.getState().collapsedChatRows).toBe(before)
    expect(saveSidebarUI).not.toHaveBeenCalled()
  })

  it('openChatRow leaves every other folded row folded', () => {
    useSidebarStore.getState().toggleChatRow('f1')
    useSidebarStore.getState().toggleChatRow('f2')
    useSidebarStore.getState().openChatRow('f1')
    expect([...useSidebarStore.getState().collapsedChatRows]).toEqual(['f2'])
  })
})
