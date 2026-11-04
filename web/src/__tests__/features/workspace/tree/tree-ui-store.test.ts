import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { IDBFactory } from 'fake-indexeddb'
import {
  parseSidebarTab,
  treeUiStore,
  type CreateDraft,
  type RemovalDraft,
} from '@/features/workspace/tree/tree-ui-store'
import { loadSidebarUI, saveSidebarUI } from '@/lib/persistence/sidebar-ui'
import { resetDB } from '@/lib/persistence/idb'
import { hydrateSidebar } from '@/lib/persistence/hydrate'

const state = () => treeUiStore.getState()
const DRAIN_MS = 8_000

function removal(over: Partial<RemovalDraft> = {}): RemovalDraft {
  return {
    kind: 'workspace',
    id: 'ws-1',
    label: 'feature/one',
    projectId: 'p1',
    repoId: 'r1',
    wsId: '',
    providerIcon: '',
    hiddenIds: ['ws-1', 'chat-1'],
    extra: 0,
    fallbackWsId: null,
    ...over,
  }
}

const draft = (over: Partial<CreateDraft> = {}): CreateDraft => ({
  kind: 'branch',
  projectId: 'p1',
  repoId: 'r1',
  workspaceId: 'ws-1',
  parentId: 'row-1',
  order: 2,
  ownsWorktree: true,
  ...over,
})

function mountRow(id: string, recents = false): HTMLElement {
  const el = document.createElement('div')
  el.setAttribute('role', 'treeitem')
  el.setAttribute('data-sidebar-row-id', id)
  if (recents) el.setAttribute('data-sidebar-recents-row', '')
  document.body.appendChild(el)
  return el
}

beforeEach(() => {
  resetDB()
  globalThis.indexedDB = new IDBFactory()
  treeUiStore.setState(treeUiStore.getInitialState(), true)
})

afterEach(() => {
  document.body.replaceChildren()
  vi.useRealTimers()
})

describe('inline rename', () => {
  it('starts with nothing renaming', () => {
    expect(state().renaming).toBeNull()
    expect(state().renamingRole).toBeNull()
  })

  it('one row at a time: a second start replaces the first', () => {
    state().startRenaming('row-1')
    state().startRenaming('row-2')
    expect(state().renaming).toBe('row-2')
  })

  it('stopRenaming clears the row and its role', () => {
    state().startRenaming('row-1')
    state().stopRenaming()
    expect(state().renaming).toBeNull()
    expect(state().renamingRole).toBeNull()
  })

  // A row in Recents and in the tree renders twice under one id; exactly one draws the editor.
  it('the tree instance draws the editor when one is mounted, even beside a Recents copy', () => {
    mountRow('row-1', true)
    mountRow('row-1')
    state().startRenaming('row-1')
    expect(state().renamingRole).toBe('tree')
  })

  it('the Recents mirror draws it when it is the only instance', () => {
    mountRow('row-1', true)
    state().startRenaming('row-1')
    expect(state().renamingRole).toBe('recents')
  })

  it('falls to Recents when no instance is mounted at all', () => {
    mountRow('row-2')
    state().startRenaming('row-1')
    expect(state().renamingRole).toBe('recents')
  })
})

describe('collapsed rows', () => {
  it('starts empty: every row is open by default', () => {
    expect(state().collapsed.size).toBe(0)
  })

  it('folds, then opens again, leaving other rows alone, in one set for folders and chats', () => {
    state().toggleChatRow('f1')
    state().toggleChatRow('c9')
    expect([...state().collapsed]).toEqual(['f1', 'c9'])
    state().toggleChatRow('f1')
    expect([...state().collapsed]).toEqual(['c9'])
  })

  it('hands out a new set so subscribers see the change', () => {
    const before = state().collapsed
    state().toggleChatRow('f1')
    expect(state().collapsed).not.toBe(before)
  })

  it('persists collapsed rows in the sidebar-ui record a production build reads', async () => {
    state().toggleChatRow('f1')
    state().toggleChatRow('c7')
    await vi.waitFor(async () =>
      expect((await loadSidebarUI())?.collapsedChatRows).toEqual(['f1', 'c7']),
    )
    const record = await loadSidebarUI()
    expect(Object.keys(record ?? {}).sort()).toEqual([
      'activeTab',
      'collapsedChatRows',
      'updatedAt',
    ])
  })

  it('a record an older build wrote, retired keys and all, folds the same rows after the upgrade', async () => {
    await saveSidebarUI({
      collapsedRepos: ['crowbar'],
      collapsedWorkspaces: ['ws3'],
      collapsedProjects: ['p2'],
      collapsedChatRows: ['f1', 'c7'],
    })
    await hydrateSidebar()
    expect([...state().collapsed]).toEqual(['f1', 'c7'])
  })

  it('a fold survives a reload, and the next write drops the retired keys', async () => {
    await saveSidebarUI({ collapsedRepos: ['crowbar'], collapsedChatRows: ['f1'] })
    await hydrateSidebar()
    state().toggleChatRow('c7')
    await vi.waitFor(async () =>
      expect((await loadSidebarUI())?.collapsedChatRows).toEqual(['f1', 'c7']),
    )
    expect((await loadSidebarUI())?.collapsedRepos).toBeUndefined()

    treeUiStore.setState(treeUiStore.getInitialState(), true)
    await hydrateSidebar()
    expect([...state().collapsed]).toEqual(['f1', 'c7'])
  })
})

describe('the removal tray', () => {
  it('hold hides every id the draft names and queues one tray row per draft, oldest first', () => {
    state().hold([removal(), removal({ id: 'f1', kind: 'folder', hiddenIds: ['f1'] })])
    expect(state().removals.map((e) => e.id)).toEqual(['ws-1', 'f1'])
    expect([...state().hiddenIds]).toEqual(['ws-1', 'chat-1', 'f1'])
    state().hold([removal({ id: 'c2', kind: 'chat', hiddenIds: ['c2'] })])
    expect(state().removals.map((e) => e.id)).toEqual(['ws-1', 'f1', 'c2'])
  })

  it('a workspace, folder or chat drains on the 8 s clock; a repo or project waits on an answer', () => {
    vi.useFakeTimers()
    vi.setSystemTime(1_000)
    const kinds = ['workspace', 'folder', 'chat', 'repo', 'project'] as const
    state().hold(kinds.map((kind) => removal({ kind, id: kind })))
    expect(state().removals.map((e) => e.deadlineAt)).toEqual([
      1_000 + DRAIN_MS,
      1_000 + DRAIN_MS,
      1_000 + DRAIN_MS,
      null,
      null,
    ])
    expect(new Set(state().removals.map((e) => e.entryId)).size).toBe(5)
  })

  it('holding nothing changes nothing', () => {
    const before = state()
    state().hold([])
    expect(state()).toBe(before)
  })

  it('cancel is the undo: the tray row goes and its rows show again, other holds stay', () => {
    state().hold([removal(), removal({ id: 'f1', kind: 'folder', hiddenIds: ['f1'] })])
    const [first] = state().removals
    state().cancel(first.entryId)
    expect(state().removals.map((e) => e.id)).toEqual(['f1'])
    expect([...state().hiddenIds]).toEqual(['f1'])
  })

  it('cancel of an entry nothing holds changes nothing', () => {
    state().hold([removal()])
    const before = state()
    state().cancel('not-a-real-entry')
    expect(state()).toBe(before)
  })

  // A sent entry leaves the tray before its tombstones land; dropping its ids early is a ghost row.
  it('settle drops the tray row but keeps every one of its ids hidden until release', () => {
    state().hold([removal()])
    const [entry] = state().removals
    state().settle(entry.entryId)
    expect(state().removals).toHaveLength(0)
    expect([...state().hiddenIds]).toEqual(['ws-1', 'chat-1'])
    state().release(entry.hiddenIds)
    expect(state().hiddenIds.size).toBe(0)
  })

  it('settle or release of something nothing holds keeps the same identities', () => {
    state().hold([removal()])
    const { removals, hiddenIds } = state()
    state().settle('not-a-real-entry')
    state().release(['nope'])
    state().release([])
    expect(state().removals).toBe(removals)
    expect(state().hiddenIds).toBe(hiddenIds)
  })

  it('askToDiscard puts a sent entry back, waiting on an answer with the work at risk', () => {
    state().hold([removal()])
    const [sent] = state().removals
    state().settle(sent.entryId)
    const atRisk = [
      { workspaceId: 'ws-1', branch: 'feature/one', uncommittedFiles: 1, unmergedCommits: 0 },
    ]
    state().askToDiscard(sent, atRisk)
    const [asked] = state().removals
    expect(asked.deadlineAt).toBeNull()
    expect(asked.atRisk).toEqual(atRisk)
    expect(asked.entryId).not.toBe(sent.entryId)
    expect(state().hiddenIds.has('chat-1')).toBe(true)
    state().cancel(asked.entryId)
    expect(state().hiddenIds.size).toBe(0)
  })

  it('askToDiscard hides the entry rows again even once they were released', () => {
    state().hold([removal()])
    const [sent] = state().removals
    state().settle(sent.entryId)
    state().release(sent.hiddenIds)
    state().askToDiscard(sent, [])
    expect([...state().hiddenIds]).toEqual(['ws-1', 'chat-1'])
  })
})

describe('the create draft', () => {
  it('startDraft opens the naming input; a second one replaces it', () => {
    state().startDraft(draft())
    state().startDraft(draft({ parentId: 'row-2' }))
    expect(state().draft).toEqual(draft({ parentId: 'row-2' }))
  })

  it('opens a folded parent, so the naming input is drawn where it can be seen', async () => {
    state().toggleChatRow('row-1')
    state().toggleChatRow('c7')
    state().startDraft(draft({ parentId: 'row-1' }))
    expect([...state().collapsed]).toEqual(['c7'])
    await vi.waitFor(async () => expect((await loadSidebarUI())?.collapsedChatRows).toEqual(['c7']))
  })

  it('cancelDraft clears the draft', () => {
    state().startDraft(draft())
    state().cancelDraft()
    expect(state().draft).toBeNull()
  })
})

describe('one store, independent parts', () => {
  it('a removal cancel leaves the draft, and a draft cancel leaves the tray', () => {
    state().startDraft(draft())
    state().hold([removal()])
    state().cancel(state().removals[0].entryId)
    expect(state().draft).toEqual(draft())
    state().hold([removal()])
    const { removals, hiddenIds } = state()
    state().cancelDraft()
    expect(state().removals).toBe(removals)
    expect(state().hiddenIds).toBe(hiddenIds)
  })

  it('a fold, a rename or a tab switch leaves every other part with its identity', () => {
    state().hold([removal()])
    const { removals, hiddenIds, collapsed } = state()
    state().toggleChatRow('f1')
    state().startRenaming('row-1')
    state().setActiveTab('git')
    expect(state().removals).toBe(removals)
    expect(state().hiddenIds).toBe(hiddenIds)
    expect(state().collapsed).not.toBe(collapsed)
    expect(state().activeTab).toBe('git')
  })
})

// The sidebar's mode is saved in the same record as the folds, so either write must carry
// the other or it would erase it.
describe('the sidebar mode', () => {
  it('defaults to Chats', () => {
    expect(state().activeTab).toBe('chats')
  })

  it('persists the mode alongside the folds', async () => {
    state().toggleChatRow('f1')
    state().setActiveTab('files')
    await vi.waitFor(async () => expect((await loadSidebarUI())?.activeTab).toBe('files'))
    expect((await loadSidebarUI())?.collapsedChatRows).toEqual(['f1'])
  })

  it('persists a fold without dropping the mode from the record', async () => {
    state().setActiveTab('git')
    state().toggleChatRow('f1')
    await vi.waitFor(async () =>
      expect((await loadSidebarUI())?.collapsedChatRows).toEqual(['f1']),
    )
    expect((await loadSidebarUI())?.activeTab).toBe('git')
  })

  it('selecting the mode already showing changes nothing', () => {
    const before = state()
    state().setActiveTab('chats')
    expect(state()).toBe(before)
  })

  it('reads a persisted mode back, and Chats for one this build has no panel for', () => {
    expect(parseSidebarTab('git')).toBe('git')
    expect(parseSidebarTab('workspaces')).toBe('chats')
    expect(parseSidebarTab(undefined)).toBe('chats')
  })
})

describe('a removal held with its own clock', () => {
  it('drains on that clock instead of the 8 s one', () => {
    vi.useFakeTimers()
    vi.setSystemTime(1_000)
    state().hold([removal({ kind: 'chat', id: 'c1' })], 0)
    expect(state().removals[0].deadlineAt).toBe(1_000)
  })
})
