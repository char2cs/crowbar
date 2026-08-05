/**
 * Contract pin: projects are OPEN by default, and folding one away is an
 * explicit, persisted act.
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
  })
})

describe('collapsedProjects', () => {
  it('starts empty — every project is OPEN by default', () => {
    expect(useSidebarStore.getState().collapsedProjects.size).toBe(0)
  })

  it('shares its polarity with collapsedRepos and collapsedWorkspaces', () => {
    // All three sets now say the same thing: membership means folded away.
    const initial = useSidebarStore.getState()
    expect(initial.collapsedRepos.size).toBe(0)
    expect(initial.collapsedWorkspaces.size).toBe(0)
    expect(initial.collapsedProjects.size).toBe(0)
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

  it('persists collapsed projects alongside the collapsed repo/workspace sets', () => {
    useSidebarStore.getState().toggleProject('p2')
    expect(saveSidebarUI).toHaveBeenCalledWith([], [], ['p2'])
  })

  it('toggleRepo and toggleWorkspace carry the collapsed projects through', () => {
    useSidebarStore.getState().toggleProject('p2')
    saveSidebarUI.mockClear()

    useSidebarStore.getState().toggleRepo('r1')
    expect(saveSidebarUI).toHaveBeenLastCalledWith(['r1'], [], ['p2'])

    useSidebarStore.getState().toggleWorkspace('w1')
    expect(saveSidebarUI).toHaveBeenLastCalledWith(['r1'], ['w1'], ['p2'])
  })
})
