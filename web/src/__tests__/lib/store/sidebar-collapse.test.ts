/**
 * Contract pin for the one set the sidebar folds away: rows are OPEN by
 * default, and folding one is an explicit, persisted act.
 *
 * `collapsedProjects`/`collapsedRepos`/`collapsedWorkspaces` are retired keys
 * the pre-restyle tree wrote: the store has no field for them and the record
 * is written without them (see project-visibility-retired-collapsed-projects
 * .test.ts and hydrate.test.ts).
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'

const { saveSidebarUI } = vi.hoisted(() => ({
  saveSidebarUI: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/lib/persistence/sidebar-ui', () => ({
  saveSidebarUI: (...args: unknown[]) => saveSidebarUI(...args),
  loadSidebarUI: vi.fn().mockResolvedValue(null),
}))

import { getInitialState, useSidebarStore } from '@/lib/store/sidebar'

beforeEach(() => {
  vi.clearAllMocks()
  useSidebarStore.setState({
    collapsedChatRows: new Set<string>(),
    activeTab: 'chats',
    hostFilter: '',
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

  it('never writes a retired key back into the record', () => {
    useSidebarStore.getState().toggleChatRow('f1')
    const record = saveSidebarUI.mock.calls[0][0] as Record<string, unknown>
    expect(Object.keys(record).sort()).toEqual(['activeTab', 'collapsedChatRows', 'hostFilter'])
  })

  it('persists the fold without dropping the mode from the record', () => {
    useSidebarStore.setState({ activeTab: 'git' })
    useSidebarStore.getState().toggleChatRow('f1')
    expect(saveSidebarUI).toHaveBeenLastCalledWith({
      collapsedChatRows: ['f1'],
      activeTab: 'git',
      hostFilter: '',
    })
  })
})

// The sidebar's mode (Chats / Files / Git) is saved in the same record as the
// folds, so either write must carry the other or it would erase it.
describe('activeTab', () => {
  it('defaults to chats', () => {
    expect(getInitialState().activeTab).toBe('chats')
  })

  it('setActiveTab persists the mode alongside the folds', () => {
    useSidebarStore.getState().toggleChatRow('f1')
    useSidebarStore.getState().setActiveTab('files')
    expect(useSidebarStore.getState().activeTab).toBe('files')
    expect(saveSidebarUI).toHaveBeenLastCalledWith({
      collapsedChatRows: ['f1'],
      activeTab: 'files',
      hostFilter: '',
    })
  })

  it('selecting the mode already showing writes nothing', () => {
    useSidebarStore.getState().setActiveTab('chats')
    expect(saveSidebarUI).not.toHaveBeenCalled()
  })
})

// The host filter shares the record with the folds and the mode, so every write carries all three.
describe('hostFilter', () => {
  it('defaults to no host chosen', () => {
    expect(getInitialState().hostFilter).toBe('')
  })

  it('setHostFilter persists the choice alongside the folds and the mode', () => {
    useSidebarStore.setState({ activeTab: 'git' })
    useSidebarStore.getState().toggleChatRow('f1')
    useSidebarStore.getState().setHostFilter('h1')
    expect(useSidebarStore.getState().hostFilter).toBe('h1')
    expect(saveSidebarUI).toHaveBeenLastCalledWith({
      collapsedChatRows: ['f1'],
      activeTab: 'git',
      hostFilter: 'h1',
    })
  })

  it('a fold or a mode change keeps the saved choice', () => {
    useSidebarStore.getState().setHostFilter('h1')
    useSidebarStore.getState().setActiveTab('files')
    expect(saveSidebarUI).toHaveBeenLastCalledWith({
      collapsedChatRows: [],
      activeTab: 'files',
      hostFilter: 'h1',
    })
  })

  it('choosing the host already chosen writes nothing', () => {
    useSidebarStore.setState({ hostFilter: 'h1' })
    useSidebarStore.getState().setHostFilter('h1')
    expect(saveSidebarUI).not.toHaveBeenCalled()
  })
})
