import { vi } from 'vitest'

// Restoring documents loads Monaco and reads disk; restore-documents.test owns that.
const restoreDocumentsMock = vi.fn(async () => {})
vi.mock('@/features/editor/documents/restore-documents', () => ({
  restoreDocuments: () => restoreDocumentsMock(),
}))

import {
  hydrateSidebar,
  hydrateWindowPaneLayout,
  placeRestoredPanesOnceSynced,
  upgradeWindowPaneLayout,
} from '@/lib/persistence/hydrate'
import { loadDrafts } from '@/features/editor/documents/draft-store'
import { getDB, resetDB } from '@/lib/persistence/idb'
import { entityStore } from '@/core/entities/entity-store'
import {
  WINDOW_LAYOUT_VERSION,
  WINDOW_SESSION_ID,
  loadWindowPaneLayout,
} from '@/lib/persistence/workspace-layout'
import type { WorkspaceLayout } from '@/lib/persistence/schemas'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import type { EditorContent } from '@/features/panes/types/pane-content'
import type { PaneGroup } from '@/features/panes/types/pane'
import { IDBFactory } from 'fake-indexeddb'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { createLeaf, getAllLeafIds } from '@/features/panes/utils/pane-layout'
import { viewIntegrityViolations } from '@/features/panes/lib/view-integrity'
import { saveSidebarUI } from '@/lib/persistence/sidebar-ui'
import type { Repo } from '@/core/entities/repo-types'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import {
  chatEntity,
  markProjectsSynced,
  putEntities,
  resetEntities,
  seedRepos,
} from '@/__tests__/__fixtures__/sidebar-entities'

/** The projects scope's snapshot landing with `chats`. */
function syncChats(chats: Array<{ id: string; workspaceId: string }>): void {
  putEntities(
    'chat',
    chats.map((c) => chatEntity(c)),
  )
  markProjectsSynced()
}

const HYDRATE_TEST_REPOS: Repo[] = [
  {
    id: 'crowbar',
    name: 'crowbar',
    avatarLabel: 'C',
    avatarColor: 'bg-indigo-700',
    workspaces: [
      { id: 'ws-develop', branch: 'develop', status: 'locked', age: '—' },
      {
        id: 'ws3',
        branch: 'feature/app-design',
        parentId: 'ws-develop',
        status: 'pr-open',
        age: '16h ago',
      },
      {
        id: 'ws1',
        branch: 'enhancement/scaffold',
        parentId: 'ws3',
        status: 'new',
        working: true,
        age: '3d ago',
      },
    ],
  },
]

describe('hydrateWindowPaneLayout', () => {
  beforeEach(() => {
    resetDB()
    globalThis.indexedDB = new IDBFactory()
    resetWindowPaneStoreForTests()
  })

  it('does nothing when IDB is empty', async () => {
    const before = windowPaneStore.getState()
    await hydrateWindowPaneLayout()
    const after = windowPaneStore.getState()
    expect(after.activePaneId).toBe(before.activePaneId)
    expect(after.panes).toEqual(before.panes)
  })

  it('restores a v2 row: views, stage, buffers that name their file, then their documents', async () => {
    const layout: WorkspaceLayout = {
      version: 2,
      workspaceId: WINDOW_SESSION_ID,
      panes: {
        [ROOT_PANE_ID]: pane(ROOT_PANE_ID, null, { editorTabIds: ['buf-1'] }),
        'pane-a': pane('pane-a', 'view-a', { chatId: 'chat-a' }),
        'pane-b': pane('pane-b', 'view-b', { chatId: 'chat-b' }),
        'bottom-pane': pane('bottom-pane', null),
      },
      views: {
        'view-a': { id: 'view-a', projectId: 'p1', layout: createLeaf('pane-a') },
        'view-b': { id: 'view-b', projectId: 'p1', layout: createLeaf('pane-b') },
      },
      viewOrder: ['view-b', 'view-a'],
      activeViewId: 'view-a',
      activeViewByProject: { p1: 'view-a' },
      stage: createLeaf(ROOT_PANE_ID),
      bottomLayout: createLeaf('bottom-pane'),
      activePaneId: 'pane-a',
      mostRecentActivePaneIds: ['pane-a'],
      buffers: [{ ...buffer('buf-1'), authority: undefined } as never],
      updatedAt: Date.now(),
    }
    await (await getDB()).put('workspace-layout', layout)
    let tabsAtRestore: string[] = []
    restoreDocumentsMock.mockImplementationOnce(async () => {
      tabsAtRestore = windowPaneStore.getState().buffers.map((b) => b.id)
    })

    await hydrateWindowPaneLayout()

    expect(tabsAtRestore).toEqual(['buf-1'])
    const state = windowPaneStore.getState()
    expect(state.viewOrder).toEqual(['view-b', 'view-a'])
    expect(state.activeViewId).toBe('view-a')
    expect(state.activePaneId).toBe('pane-a')
    expect(getAllLeafIds(state.stage)).toEqual([ROOT_PANE_ID])
    expect(state.buffers).toMatchObject([{ id: 'buf-1', authority: 'workspace' }])
    expect(state.buffers[0]).not.toHaveProperty('content')
  })

  // A v1 member carried only its chat; it is restored unplaced, and the projects scope
  // places it, or drops it once a snapshot that read every repo leaves its chat out.
  it('upgrades a v1 layout: members gain their workspace once the projects scope answers', async () => {
    resetEntities()
    const db = await getDB()
    const v1 = (id: string, viewId: string, chatId: string) => {
      const p = pane(id, viewId, { chatId })
      delete p.workspaceId
      return p
    }
    await db.put('workspace-layout', {
      workspaceId: WINDOW_SESSION_ID,
      panes: {
        'pane-a': v1('pane-a', 'view-a', 'chat-a'),
        'pane-x': v1('pane-x', 'view-x', 'chat-unknown'),
        'bottom-pane': pane('bottom-pane', null),
      },
      views: {
        'view-a': { id: 'view-a', projectId: 'p1', layout: createLeaf('pane-a') },
        'view-x': { id: 'view-x', projectId: 'p1', layout: createLeaf('pane-x') },
      },
      viewOrder: ['view-a', 'view-x'],
      activeViewId: 'view-a',
      stage: createLeaf(ROOT_PANE_ID),
      bottomLayout: createLeaf('bottom-pane'),
      activePaneId: 'pane-a',
      mostRecentActivePaneIds: ['pane-a'],
      buffers: [],
      updatedAt: Date.now(),
    } as WorkspaceLayout)
    await hydrateWindowPaneLayout()
    placeRestoredPanesOnceSynced()
    const placed = () => windowPaneStore.getState().panes
    const views = () => windowPaneStore.getState().viewOrder
    expect(views()).toEqual(['view-a', 'view-x'])
    expect([placed()['pane-a'].workspaceId, placed()['pane-x'].workspaceId]).toEqual([null, null])
    // One that could not read some repo places, but says no chat is gone.
    entityStore.setState({ unread: { projects: ['r-unread'] } })
    syncChats([{ id: 'chat-a', workspaceId: 'ws-a' }])
    expect(placed()['pane-a'].workspaceId).toBe('ws-a')
    expect(views()).toEqual(['view-a', 'view-x'])
    entityStore.setState({ unread: { projects: [] } })
    expect(views()).toEqual(['view-a'])
  })

  it('a payload without views hydrates to an empty band and keeps its buffers', async () => {
    const db = await getDB()
    await db.put('workspace-layout', {
      workspaceId: WINDOW_SESSION_ID,
      panes: { [ROOT_PANE_ID]: pane(ROOT_PANE_ID, null) },
      rootLayout: createLeaf(ROOT_PANE_ID),
      dormantArrangements: [{ id: 'entry-a', chatIds: ['chat-a'], state: 'dormant' }],
      bottomLayout: createLeaf('bottom-pane'),
      activePaneId: ROOT_PANE_ID,
      mostRecentActivePaneIds: [ROOT_PANE_ID],
      buffers: [buffer('buf-1')],
      updatedAt: Date.now(),
    } as unknown as WorkspaceLayout)

    await hydrateWindowPaneLayout()

    const state = windowPaneStore.getState()
    expect(state.viewOrder).toEqual([])
    expect(state.views).toEqual({})
    expect(state.activeViewId).toBeNull()
    // As the base build did: the buffers come back, held by the stage.
    expect(state.buffers.map((b) => b.id)).toEqual(['buf-1'])
    expect(state.panes[getAllLeafIds(state.stage)[0]].editorTabIds).toEqual(['buf-1'])
  })

  it('a payload whose only record is broken hydrates to an empty band', async () => {
    const db = await getDB()
    await db.put('workspace-layout', {
      workspaceId: WINDOW_SESSION_ID,
      panes: { 'pane-a': pane('pane-a', 'view-a', { chatId: null }) },
      views: { 'view-a': { id: 'view-a', projectId: 'p1', layout: createLeaf('pane-a') } },
      viewOrder: ['view-a'],
      activeViewId: 'view-a',
      stage: createLeaf(ROOT_PANE_ID),
      bottomLayout: createLeaf('bottom-pane'),
      activePaneId: 'pane-a',
      mostRecentActivePaneIds: [],
      buffers: [],
      updatedAt: Date.now(),
    })

    await hydrateWindowPaneLayout()

    expect(windowPaneStore.getState().viewOrder).toEqual([])
    expect(windowPaneStore.getState().activeViewId).toBeNull()
  })

  // One broken record must not discard the whole band.
  it('repairs a payload with one broken record, keeping every valid one in order', async () => {
    const db = await getDB()
    await db.put('workspace-layout', {
      workspaceId: WINDOW_SESSION_ID,
      panes: {
        [ROOT_PANE_ID]: pane(ROOT_PANE_ID, null),
        'pane-a': pane('pane-a', 'view-a', { chatId: 'chat-a' }),
        'pane-x': pane('pane-x', 'view-x', { chatId: null }),
        'pane-b': pane('pane-b', 'view-b', { chatId: 'chat-b' }),
        'bottom-pane': pane('bottom-pane', null),
      },
      views: {
        'view-a': { id: 'view-a', projectId: 'p1', layout: createLeaf('pane-a') },
        'view-x': { id: 'view-x', projectId: 'p1', layout: createLeaf('pane-x') },
        'view-b': { id: 'view-b', projectId: 'p1', layout: createLeaf('pane-b') },
      },
      viewOrder: ['view-b', 'view-x', 'view-a', 'view-gone'],
      activeViewId: 'view-x',
      activeViewByProject: { p1: 'view-x' },
      stage: createLeaf(ROOT_PANE_ID),
      bottomLayout: createLeaf('bottom-pane'),
      activePaneId: 'pane-x',
      mostRecentActivePaneIds: ['pane-x', 'pane-a'],
      buffers: [],
      updatedAt: Date.now(),
    })

    await hydrateWindowPaneLayout()

    const state = windowPaneStore.getState()
    expect(state.viewOrder).toEqual(['view-b', 'view-a'])
    expect(Object.keys(state.views).sort()).toEqual(['view-a', 'view-b'])
    expect(state.panes['pane-x']).toBeUndefined()
    expect(viewIntegrityViolations(state)).toEqual([])
  })
})

/**
 * First load after upgrading an install from the base build (70ec430). The
 * seeds are exactly what that build wrote: a layout row with no `version`,
 * chat panes with no `workspaceId`, the retired `sidebarWidth` fields, and
 * whatever buffers it held (it kept buffers no pane listed).
 */
describe('hydrateWindowPaneLayout — upgrade from the base build', () => {
  const P = 'p1'

  beforeEach(() => {
    resetDB()
    globalThis.indexedDB = new IDBFactory()
    resetWindowPaneStoreForTests()
    resetEntities()
  })

  function basePane(id: string, viewId: string | null, over: Partial<PaneGroup> = {}): PaneGroup {
    const p = pane(id, viewId, over)
    delete p.workspaceId
    return p
  }

  function dirty(id: string, workspaceId: string): EditorContent {
    return {
      ...buffer(id),
      path: `/src/${id}.ts`,
      content: 'unsaved edits',
      savedContent: 'on disk',
      isDirty: true,
      workspaceId,
    } as EditorContent
  }

  async function seedBaseLayout(over: Record<string, unknown>) {
    const db = await getDB()
    await db.put('workspace-layout', {
      workspaceId: WINDOW_SESSION_ID,
      panes: {},
      views: {},
      viewOrder: [],
      activeViewId: null,
      activeViewByProject: {},
      stage: createLeaf(ROOT_PANE_ID),
      bottomLayout: createLeaf('bottom-pane'),
      activePaneId: ROOT_PANE_ID,
      mostRecentActivePaneIds: [],
      buffers: [],
      sidebarWidth: 0,
      rightSidebarWidth: 0,
      updatedAt: Date.now(),
      ...over,
    } as unknown as WorkspaceLayout)
  }

  it('keeps an unplaced chat pane with its dirty buffer until the projects scope places it', async () => {
    await seedBaseLayout({
      panes: {
        [ROOT_PANE_ID]: basePane(ROOT_PANE_ID, null),
        'pane-home': basePane('pane-home', 'view-home', {
          chatId: 'chat-home',
          editorTabIds: ['buf-dirty'],
          activeEditorTabId: 'buf-dirty',
          editorOpen: true,
        }),
        'pane-repo': basePane('pane-repo', 'view-repo', { chatId: 'chat-repo' }),
        'bottom-pane': basePane('bottom-pane', null),
      },
      views: {
        'view-home': { id: 'view-home', projectId: P, layout: createLeaf('pane-home') },
        'view-repo': { id: 'view-repo', projectId: P, layout: createLeaf('pane-repo') },
      },
      viewOrder: ['view-home', 'view-repo'],
      activeViewId: 'view-home',
      activeViewByProject: { [P]: 'view-home' },
      activePaneId: 'pane-home',
      mostRecentActivePaneIds: ['pane-home'],
      // `buf-orphan` is listed by no pane — the base build leaked it, unsaved.
      buffers: [dirty('buf-dirty', 'ws-home'), dirty('buf-orphan', 'ws-repo')],
    })

    await hydrateWindowPaneLayout()

    const state = windowPaneStore.getState()
    expect(state.viewOrder).toEqual(['view-home', 'view-repo'])
    expect(state.activeViewId).toBe('view-home')
    expect(state.panes['pane-home']).toMatchObject({
      chatId: 'chat-home',
      workspaceId: null,
      editorTabIds: ['buf-dirty'],
    })
    expect(state.panes['pane-repo'].workspaceId).toBeNull()
    // No unsaved edit is lost: the orphan is re-homed to a surviving pane.
    expect(state.buffers.map((b) => b.id).sort()).toEqual(['buf-dirty', 'buf-orphan'])
    const holders = Object.values(state.panes).filter((p) => p.editorTabIds.includes('buf-orphan'))
    expect(holders).toHaveLength(1)
    expect(state.buffers.every((b) => (b as EditorContent).isDirty)).toBe(true)
    expect([...(await loadDrafts()).values()].map((d) => d.text)).toEqual([
      'unsaved edits',
      'unsaved edits',
    ])
    expect(viewIntegrityViolations(state)).toEqual([])

    // The projects scope then places both members.
    placeRestoredPanesOnceSynced()
    syncChats([
      { id: 'chat-home', workspaceId: 'ws-home' },
      { id: 'chat-repo', workspaceId: 'ws-repo' },
    ])

    const placed = windowPaneStore.getState()
    expect(placed.panes['pane-home'].workspaceId).toBe('ws-home')
    expect(placed.panes['pane-repo'].workspaceId).toBe('ws-repo')
    expect(placed.panes['pane-home'].editorTabIds).toEqual(['buf-dirty'])
    expect(placed.viewOrder).toEqual(['view-home', 'view-repo'])
  })

  it('drops a member the projects scope does not carry, re-homing its unsaved buffer', async () => {
    await seedBaseLayout({
      panes: {
        [ROOT_PANE_ID]: basePane(ROOT_PANE_ID, null),
        'pane-gone': basePane('pane-gone', 'view-gone', {
          chatId: 'chat-gone',
          editorTabIds: ['buf-dirty', 'buf-clean'],
          activeEditorTabId: 'buf-dirty',
          editorOpen: true,
        }),
        'pane-kept': basePane('pane-kept', 'view-kept', { chatId: 'chat-kept' }),
        'bottom-pane': basePane('bottom-pane', null),
      },
      views: {
        'view-gone': { id: 'view-gone', projectId: P, layout: createLeaf('pane-gone') },
        'view-kept': { id: 'view-kept', projectId: P, layout: createLeaf('pane-kept') },
      },
      viewOrder: ['view-gone', 'view-kept'],
      activeViewId: 'view-kept',
      activeViewByProject: { [P]: 'view-kept' },
      activePaneId: 'pane-kept',
      buffers: [dirty('buf-dirty', 'ws-x'), { ...buffer('buf-clean'), workspaceId: 'ws-x' }],
    })
    await hydrateWindowPaneLayout()

    syncChats([{ id: 'chat-kept', workspaceId: 'ws-k' }])
    placeRestoredPanesOnceSynced()

    const state = windowPaneStore.getState()
    expect(state.viewOrder).toEqual(['view-kept'])
    expect(state.panes['pane-gone']).toBeUndefined()
    expect(state.panes['pane-kept'].workspaceId).toBe('ws-k')
    // The unsaved buffer outlives its chat; the clean one is released.
    expect(state.buffers.map((b) => b.id)).toEqual(['buf-dirty'])
    const holders = Object.values(state.panes).filter((p) => p.editorTabIds.includes('buf-dirty'))
    expect(holders).toHaveLength(1)
    expect(viewIntegrityViolations(state)).toEqual([])
  })

  it('keeps an unknown member unplaced until the projects scope answers', async () => {
    await seedBaseLayout({
      panes: {
        'pane-a': basePane('pane-a', 'view-a', { chatId: 'chat-a' }),
        'bottom-pane': basePane('bottom-pane', null),
      },
      views: { 'view-a': { id: 'view-a', projectId: P, layout: createLeaf('pane-a') } },
      viewOrder: ['view-a'],
      activeViewId: 'view-a',
      activePaneId: 'pane-a',
    })
    await hydrateWindowPaneLayout()

    placeRestoredPanesOnceSynced()

    const state = windowPaneStore.getState()
    expect(state.viewOrder).toEqual(['view-a'])
    expect(state.panes['pane-a'].workspaceId).toBeNull()
  })

  it('stamps the upgraded layout once; a second load does not upgrade again', async () => {
    await seedBaseLayout({
      panes: {
        'pane-a': basePane('pane-a', 'view-a', { chatId: 'chat-a' }),
        'bottom-pane': basePane('bottom-pane', null),
      },
      views: { 'view-a': { id: 'view-a', projectId: P, layout: createLeaf('pane-a') } },
      viewOrder: ['view-a'],
      activeViewId: 'view-a',
      activePaneId: 'pane-a',
    })

    await hydrateWindowPaneLayout()

    const stamped = await loadWindowPaneLayout()
    expect(stamped?.version).toBe(WINDOW_LAYOUT_VERSION)
    expect(stamped?.panes['pane-a']).toMatchObject({ chatId: 'chat-a', workspaceId: null })
    // Idempotent: upgrading an upgraded layout changes nothing.
    expect(await upgradeWindowPaneLayout(stamped!)).toBe(stamped)
    resetWindowPaneStoreForTests()
    await hydrateWindowPaneLayout()
    expect(windowPaneStore.getState().panes['pane-a'].workspaceId).toBeNull()
  })

  it('a base layout without views restores its buffers', async () => {
    await seedBaseLayout({
      views: undefined,
      viewOrder: undefined,
      panes: { [ROOT_PANE_ID]: basePane(ROOT_PANE_ID, null, { editorTabIds: ['buf-1'] }) },
      buffers: [dirty('buf-1', 'ws-a'), { ...buffer('buf-2'), workspaceId: 'ws-a' }],
    })

    await hydrateWindowPaneLayout()

    const state = windowPaneStore.getState()
    expect(state.buffers.map((b) => b.id)).toEqual(['buf-1', 'buf-2'])
    expect(state.panes[ROOT_PANE_ID].editorTabIds).toEqual(['buf-1', 'buf-2'])
    expect(state.buffers[0]).toMatchObject({ isDirty: true })
    expect((await loadDrafts()).size).toBe(1)
  })
})

function pane(
  id: string,
  viewId: string | null,
  over: Partial<WorkspaceLayout['panes'][string]> = {},
): WorkspaceLayout['panes'][string] {
  return {
    id,
    type: 'group',
    chatId: null,
    runnerId: null,
    editorTabIds: [],
    activeEditorTabId: over.editorTabIds?.[0] ?? null,
    editorOpen: false,
    // A current-shape member records its workspace.
    workspaceId: over.chatId ? `ws-of-${over.chatId}` : null,
    ...over,
    viewId,
  }
}

/** A buffer as layouts before v4 saved it, text and all. */
function buffer(id: string): EditorContent {
  return {
    id,
    type: 'editor',
    authority: 'workspace',
    path: '/src/main.ts',
    name: 'main.ts',
    content: 'saved',
    savedContent: 'saved',
    isDirty: false,
    isVirtual: false,
    isPreview: false,
    tokens: [],
    workspaceId: 'ws-test',
  } as EditorContent
}

describe('hydrateSidebar', () => {
  beforeEach(async () => {
    resetDB()
    globalThis.indexedDB = new IDBFactory()
    seedRepos(HYDRATE_TEST_REPOS.map((r) => ({ ...r, workspaces: [...r.workspaces] })))
    treeUiStore.setState(treeUiStore.getInitialState(), true)
  })

  // A mode an older build wrote ('workspaces'), or none, must not name a missing panel.
  it.each([
    ['files', 'files'],
    ['workspaces', 'chats'],
    [undefined, 'chats'],
  ])('replays the persisted sidebar mode %s as %s', async (activeTab, mode) => {
    treeUiStore.setState({ activeTab: 'git' })
    await saveSidebarUI({ collapsedChatRows: [], activeTab })
    await hydrateSidebar()
    expect(treeUiStore.getState().activeTab).toBe(mode)
  })

  it('does nothing when IDB is empty', async () => {
    await hydrateSidebar()
    expect(treeUiStore.getState().collapsed.size).toBe(0)
  })

  // Retired keys a previous build persisted replay as "everything open", the product
  // default: gating on them left repos that never loaded a thread and projects blank.
  it('ignores the retired collapsedRepos, -Workspaces and -Projects keys', async () => {
    await saveSidebarUI({
      collapsedRepos: ['crowbar', 'quiver-core'],
      collapsedWorkspaces: ['ws3', 'ws1'],
      collapsedProjects: ['p2', 'p3'],
      collapsedChatRows: ['f1'],
    })
    await hydrateSidebar()
    const state = treeUiStore.getState() as unknown as Record<string, unknown>
    expect(state.collapsedRepos ?? state.collapsedWorkspaces ?? state.collapsedProjects).toBe(
      undefined,
    )
    expect([...treeUiStore.getState().collapsed]).toEqual(['f1'])
  })

  it('restores collapsedChatRows from IDB', async () => {
    await saveSidebarUI({ collapsedRepos: [], collapsedChatRows: ['f1', 'c7'] })
    await hydrateSidebar()
    expect([...treeUiStore.getState().collapsed]).toEqual(['f1', 'c7'])
  })

  it('replays a record written before the Chats panel was collapsible as "all open"', async () => {
    treeUiStore.setState({ collapsed: new Set(['stale']) })
    await saveSidebarUI({ collapsedRepos: ['crowbar'], collapsedWorkspaces: [] })
    await hydrateSidebar()
    expect(treeUiStore.getState().collapsed.size).toBe(0)
  })
})
