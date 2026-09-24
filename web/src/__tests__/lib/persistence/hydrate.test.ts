import { vi } from 'vitest'

const readFileMock = vi.fn<(wsId: string, path: string) => Promise<string>>()
vi.mock('@/features/file-system/controllers/platform', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('@/features/file-system/controllers/platform')>()
  return { ...actual, readWorkspaceFile: (wsId: string, path: string) => readFileMock(wsId, path) }
})

const toastWarning = vi.fn()
vi.mock('@/features/window/stores/toast-store', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/features/window/stores/toast-store')>()
  return {
    ...actual,
    toast: {
      ...actual.toast,
      warning: (...args: unknown[]) => toastWarning(...args),
    },
  }
})

import {
  hydrateWorkspace,
  hydrateSidebar,
  hydrateWindowPaneLayout,
} from '@/lib/persistence/hydrate'
import { ApiError } from '@/lib/api'
import { getDB, resetDB } from '@/lib/persistence/idb'
import { WINDOW_SESSION_ID } from '@/lib/persistence/workspace-layout'
import type { WorkspaceLayout, UIPreferences, EditorState } from '@/lib/persistence/schemas'
import { destroyWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import type { EditorContent } from '@/features/panes/types/pane-content'
import { IDBFactory } from 'fake-indexeddb'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { upsertEntity } from '@/lib/persistence/entity-cache'
import type { ChatDTO } from '@/lib/types'
import { createLeaf, getAllLeafIds } from '@/features/panes/utils/pane-layout'
import { viewIntegrityViolations } from '@/features/panes/lib/view-integrity'
import { saveSidebarUI } from '@/lib/persistence/sidebar-ui'
import { useSidebarStore } from '@/lib/store/sidebar'
import type { Repo } from '@/lib/store/sidebar'

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

async function seedDB(workspaceId: string) {
  const db = await getDB()
  const layout: WorkspaceLayout = {
    workspaceId,
    panes: {
      [ROOT_PANE_ID]: {
        id: ROOT_PANE_ID,
        type: 'group',
        chatId: null,
        runnerId: null,
        editorTabIds: [],
        activeEditorTabId: null,
        editorOpen: false,
        viewId: null,
      },
    },
    views: {},
    viewOrder: [],
    activeViewId: null,
    stage: createLeaf(ROOT_PANE_ID),
    bottomLayout: createLeaf('bottom-pane'),
    activePaneId: ROOT_PANE_ID,
    mostRecentActivePaneIds: [ROOT_PANE_ID],
    buffers: [],
    updatedAt: Date.now(),
  }
  const prefs: UIPreferences = {
    theme: 'dark',
    fontSize: 14,
    fontFamily: 'Geist Mono',
    tabSize: 2,
    wordWrap: false,
    minimap: true,
    updatedAt: Date.now(),
  }
  const editorState: EditorState = {
    workspaceId,
    bufferId: '/src/main.ts',
    cursorLine: 10,
    cursorColumn: 5,
    scrollTop: 200,
    folds: [],
    updatedAt: Date.now(),
  }
  await db.put('workspace-layout', layout)
  await db.put('ui-preferences', prefs, 'global')
  await db.put('editor-state', editorState)
  return { layout, prefs, editorState }
}

describe('hydrateWorkspace', () => {
  beforeEach(async () => {
    resetDB()
    globalThis.indexedDB = new IDBFactory()
  })

  afterEach(() => {
    destroyWorkspaceStore('missing-ws')
    destroyWorkspaceStore('ws-test')
  })

  // Task 26: hydrateWorkspace no longer returns `layout` — pane/buffer layout
  // is window-level now and restored once at boot by hydrateWindowPaneLayout
  // (see the describe block below); this is left with editorStates only
  // (still workspace+buffer keyed, untouched by the hoist).
  it('returns empty editor states when IDB is empty', async () => {
    const result = await hydrateWorkspace('missing-ws')
    expect(result.editorStates).toEqual([])
  })

  it('returns editor states when seeded', async () => {
    const { editorState } = await seedDB('ws-test')
    const result = await hydrateWorkspace('ws-test')
    expect(result.editorStates).toHaveLength(1)
    expect(result.editorStates[0].bufferId).toBe(editorState.bufferId)
  })
})

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

  it('restores the views, the stage and the buffers from the one window-session row', async () => {
    const db = await getDB()
    const layout: WorkspaceLayout = {
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
      buffers: [buffer('buf-1')],
      updatedAt: Date.now(),
    }
    await db.put('workspace-layout', layout)

    await hydrateWindowPaneLayout()

    const state = windowPaneStore.getState()
    expect(state.viewOrder).toEqual(['view-b', 'view-a'])
    expect(state.activeViewId).toBe('view-a')
    expect(state.activePaneId).toBe('pane-a')
    expect(getAllLeafIds(state.stage)).toEqual([ROOT_PANE_ID])
    expect(state.buffers).toHaveLength(1)
    expect(state.buffers[0]).toMatchObject({ id: 'buf-1', content: 'saved' })
  })

  // The one versioned load-time upgrade: a v1 member carried only its chat;
  // v2 records the workspace, read from the chat's own cached record.
  it('upgrades a v1 layout: members gain their workspace from the chat cache', async () => {
    await upsertEntity('crowbar_chats', {
      id: 'chat-a',
      repoId: 'r1',
      projectId: 'p1',
      workspaceId: 'ws-a',
      title: 'A',
    } as ChatDTO)
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

    const state = windowPaneStore.getState()
    expect(state.panes['pane-a'].workspaceId).toBe('ws-a')
    // A member the cache cannot place is not guessed at: it, and the view it
    // leaves empty, are not restored.
    expect(state.viewOrder).toEqual(['view-a'])
  })

  it('a payload without views hydrates to an empty band; unlisted buffers are not restored', async () => {
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
    expect(state.buffers).toHaveLength(0)
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

  // Regression: one violation used to discard the whole band.
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
    // A current-shape member records its workspace (C3).
    workspaceId: over.chatId ? `ws-of-${over.chatId}` : null,
    ...over,
    viewId,
  }
}

function buffer(id: string): EditorContent {
  return {
    id,
    type: 'editor',
    path: '/src/main.ts',
    name: 'main.ts',
    content: 'saved',
    savedContent: 'saved',
    isDirty: false,
    isVirtual: false,
    isPinned: false,
    isPreview: false,
    tokens: [],
    workspaceId: 'ws-test',
  } as EditorContent
}

describe('hydrateWorkspace — restored buffer reconciliation (BUG-026/BUG-013)', () => {
  const WS = 'ws-restore'

  function makeEditorBuffer(overrides: Partial<EditorContent> = {}): EditorContent {
    return {
      id: 'buf-1',
      type: 'editor',
      path: '/repo/README.md',
      name: 'README.md',
      content: 'saved content',
      savedContent: 'saved content',
      isDirty: false,
      isVirtual: false,
      isPinned: false,
      isPreview: false,
      tokens: [],
      workspaceId: WS,
      ...overrides,
    }
  }

  // Task 26: buffers are window-level now, restored from the one
  // WINDOW_SESSION_ID-keyed IDB row by hydrateWindowPaneLayout() (once at
  // boot) — not the per-workspace `hydrateWorkspace` under test here, which
  // only reconciles whatever hydrateWindowPaneLayout already restored against
  // disk. Route through the real restore path (IDB write + a real
  // hydrateWindowPaneLayout() call) rather than poking `windowPaneStore`
  // directly, so restoreBufferDirtyState's persisted-isDirty correction is
  // still genuinely exercised, not bypassed.
  async function seedLayoutWithBuffers(buffers: EditorContent[]) {
    resetWindowPaneStoreForTests()
    const db = await getDB()
    await db.put('workspace-layout', {
      workspaceId: WINDOW_SESSION_ID,
      panes: {
        [ROOT_PANE_ID]: {
          id: ROOT_PANE_ID,
          type: 'group',
          chatId: null,
          runnerId: null,
          editorTabIds: buffers.map((b) => b.id),
          activeEditorTabId: buffers[0]?.id ?? null,
          editorOpen: true,
          viewId: null,
        },
      },
      views: {},
      viewOrder: [],
      activeViewId: null,
      stage: createLeaf(ROOT_PANE_ID),
      bottomLayout: createLeaf('bottom-pane'),
      activePaneId: ROOT_PANE_ID,
      mostRecentActivePaneIds: [ROOT_PANE_ID],
      buffers,
      updatedAt: Date.now(),
    })
    await hydrateWindowPaneLayout()
  }

  function getRestoredBuffer(): EditorContent {
    return windowPaneStore.getState().buffers[0] as EditorContent
  }

  beforeEach(() => {
    resetDB()
    globalThis.indexedDB = new IDBFactory()
    readFileMock.mockReset()
    toastWarning.mockReset()
    localStorage.removeItem(`workspace:${WS}:state`)
  })

  afterEach(() => {
    destroyWorkspaceStore(WS)
  })

  it('silently reloads a restored clean buffer whose file changed on disk', async () => {
    await seedLayoutWithBuffers([makeEditorBuffer()])
    readFileMock.mockResolvedValue('disk content changed while closed')

    await hydrateWorkspace(WS)

    const buf = getRestoredBuffer()
    // Reads must target the hydrating workspace explicitly, not the active one.
    expect(readFileMock).toHaveBeenCalledWith(WS, '/repo/README.md')
    expect(buf.content).toBe('disk content changed while closed')
    expect(buf.savedContent).toBe('disk content changed while closed')
    expect(buf.isDirty).toBe(false)
    expect(buf.hasExternalChange).toBe(false)
    expect(toastWarning).not.toHaveBeenCalled()
  })

  it('keeps a restored dirty buffer intact and flags the external change', async () => {
    await seedLayoutWithBuffers([
      makeEditorBuffer({ content: 'unsaved user edits', isDirty: true }),
    ])
    readFileMock.mockResolvedValue('disk content changed while closed')

    await hydrateWorkspace(WS)

    const buf = getRestoredBuffer()
    expect(buf.content).toBe('unsaved user edits')
    expect(buf.isDirty).toBe(true)
    expect(buf.hasExternalChange).toBe(true)
    expect(toastWarning).toHaveBeenCalledTimes(1)
  })

  it('restores the dirty marker when content diverges from savedContent even if isDirty was persisted as false', async () => {
    await seedLayoutWithBuffers([
      makeEditorBuffer({ content: 'unsaved user edits', isDirty: false }),
    ])
    // Disk matches savedContent — no external change, so no toast/reload.
    readFileMock.mockResolvedValue('saved content')

    await hydrateWorkspace(WS)

    const buf = getRestoredBuffer()
    expect(buf.content).toBe('unsaved user edits')
    expect(buf.isDirty).toBe(true)
    expect(toastWarning).not.toHaveBeenCalled()
  })

  it('does not reload or toast when disk matches the restored buffer', async () => {
    await seedLayoutWithBuffers([makeEditorBuffer()])
    readFileMock.mockResolvedValue('saved content')

    await hydrateWorkspace(WS)

    const buf = getRestoredBuffer()
    expect(buf.content).toBe('saved content')
    expect(buf.isDirty).toBe(false)
    expect(buf.hasExternalChange).toBeUndefined()
    expect(toastWarning).not.toHaveBeenCalled()
  })

  it('skips virtual editor buffers and survives unreadable files', async () => {
    await seedLayoutWithBuffers([
      makeEditorBuffer({ id: 'buf-virtual', path: 'untitled:1', isVirtual: true }),
    ])
    readFileMock.mockRejectedValue(new Error('ENOENT'))

    await hydrateWorkspace(WS)

    expect(readFileMock).not.toHaveBeenCalled()
    expect(getRestoredBuffer().content).toBe('saved content')
    expect(toastWarning).not.toHaveBeenCalled()
  })

  // BUG-001: a restored tab for a file that no longer exists must become a
  // terminal "file not found" state — flagged once, never re-fetched.
  it('flags the buffer fileMissing when the content load 404s', async () => {
    await seedLayoutWithBuffers([makeEditorBuffer()])
    readFileMock.mockRejectedValue(new ApiError('file not found', 404))

    await hydrateWorkspace(WS)

    const buf = getRestoredBuffer()
    expect(readFileMock).toHaveBeenCalledTimes(1)
    expect(buf.fileMissing).toBe(true)
    // Content untouched — closing the tab or restoring the file is the way out.
    expect(buf.content).toBe('saved content')
  })

  it('keeps the editor (no fileMissing) for a dirty buffer whose file 404s', async () => {
    await seedLayoutWithBuffers([
      makeEditorBuffer({ content: 'unsaved user edits', isDirty: true }),
    ])
    readFileMock.mockRejectedValue(new ApiError('file not found', 404))

    await hydrateWorkspace(WS)

    const buf = getRestoredBuffer()
    // The unsaved edits are the only copy left; saving recreates the file.
    expect(buf.fileMissing).toBeUndefined()
    expect(buf.content).toBe('unsaved user edits')
  })

  it('does not flag fileMissing on non-404 errors (transient failures)', async () => {
    await seedLayoutWithBuffers([makeEditorBuffer()])
    readFileMock.mockRejectedValue(new ApiError('backend exploded', 500))

    await hydrateWorkspace(WS)

    expect(getRestoredBuffer().fileMissing).toBeUndefined()
  })

  it('clears a persisted fileMissing flag when the file is back on disk', async () => {
    await seedLayoutWithBuffers([makeEditorBuffer({ fileMissing: true })])
    readFileMock.mockResolvedValue('saved content')

    await hydrateWorkspace(WS)

    expect(getRestoredBuffer().fileMissing).toBe(false)
  })
})

// Keep-alive warm return: the workspace store is LIVE (no hydration happens),
// but files changed on disk while the workspace sat hidden — its file watcher
// is active-only and agents keep editing hidden worktrees. The warm-activation
// path calls this to apply the same policy as the restore-time reconcile.
describe('reconcileWorkspaceBuffersWithDisk (keep-alive warm return)', () => {
  const WS = 'ws-warm'

  function liveEditorBuffer(overrides: Partial<EditorContent> = {}): EditorContent {
    return {
      id: 'buf-live',
      type: 'editor',
      path: '/repo/agent-edited.ts',
      name: 'agent-edited.ts',
      content: 'content when hidden',
      savedContent: 'content when hidden',
      isDirty: false,
      isVirtual: false,
      isPinned: false,
      isPreview: false,
      tokens: [],
      workspaceId: WS,
      ...overrides,
    }
  }

  beforeEach(() => {
    resetDB()
    globalThis.indexedDB = new IDBFactory()
    readFileMock.mockReset()
    toastWarning.mockReset()
    localStorage.removeItem(`workspace:${WS}:state`)
    resetWindowPaneStoreForTests()
  })

  afterEach(() => {
    destroyWorkspaceStore(WS)
  })

  it('reloads a clean buffer whose file changed on disk while the workspace was hidden', async () => {
    windowPaneStore.setState((s) => {
      s.buffers = [liveEditorBuffer()]
      return s
    })
    readFileMock.mockResolvedValue('agent rewrote this while you were away')

    const { reconcileWorkspaceBuffersWithDisk } = await import('@/lib/persistence/hydrate')
    await reconcileWorkspaceBuffersWithDisk(WS)

    const buf = windowPaneStore.getState().buffers[0] as EditorContent
    expect(readFileMock).toHaveBeenCalledWith(WS, '/repo/agent-edited.ts')
    expect(buf.content).toBe('agent rewrote this while you were away')
    expect(buf.savedContent).toBe('agent rewrote this while you were away')
    expect(buf.isDirty).toBe(false)
  })

  it('keeps a dirty buffer intact and flags the external change', async () => {
    windowPaneStore.setState((s) => {
      s.buffers = [liveEditorBuffer({ content: 'unsaved user edits', isDirty: true })]
      return s
    })
    readFileMock.mockResolvedValue('agent rewrote this while you were away')

    const { reconcileWorkspaceBuffersWithDisk } = await import('@/lib/persistence/hydrate')
    await reconcileWorkspaceBuffersWithDisk(WS)

    const buf = windowPaneStore.getState().buffers[0] as EditorContent
    expect(buf.content).toBe('unsaved user edits')
    expect(buf.isDirty).toBe(true)
    expect(buf.hasExternalChange).toBe(true)
  })

  it('is a no-op when disk still matches the buffers', async () => {
    windowPaneStore.setState((s) => {
      s.buffers = [liveEditorBuffer()]
      return s
    })
    readFileMock.mockResolvedValue('content when hidden')

    const { reconcileWorkspaceBuffersWithDisk } = await import('@/lib/persistence/hydrate')
    await reconcileWorkspaceBuffersWithDisk(WS)

    const buf = windowPaneStore.getState().buffers[0] as EditorContent
    expect(buf.content).toBe('content when hidden')
    expect(buf.hasExternalChange).toBeUndefined()
    expect(toastWarning).not.toHaveBeenCalled()
  })
})

describe('hydrateSidebar', () => {
  beforeEach(async () => {
    resetDB()
    globalThis.indexedDB = new IDBFactory()
    useSidebarStore.setState({
      repos: HYDRATE_TEST_REPOS.map((r) => ({ ...r, workspaces: [...r.workspaces] })),
      collapsedChatRows: new Set<string>(),
      activeTab: 'workspaces',
    })
  })

  it('does nothing when IDB is empty', async () => {
    await hydrateSidebar()
    expect(useSidebarStore.getState().collapsedChatRows.size).toBe(0)
  })

  // REGRESSION (restyle v2): the previous build's tree collapsed every repo
  // but the active one and persisted that set; the restyled tree folds via
  // `collapsedChatRows` and never writes it, yet the sync engine still gated
  // a repo's chats/folders on it. Over an existing profile those repos drew
  // header + branch rows and never loaded a thread. The retired key replays
  // as "every repo open", the product default.
  it('ignores the retired collapsedRepos key a previous build persisted', async () => {
    await saveSidebarUI({
      collapsedRepos: ['crowbar', 'quiver-core'],
      collapsedWorkspaces: [],
      collapsedChatRows: ['f1'],
    })
    await hydrateSidebar()
    const state = useSidebarStore.getState() as unknown as Record<string, unknown>
    expect(state.collapsedRepos).toBeUndefined()
    expect(useSidebarStore.getState().collapsedChatRows.has('f1')).toBe(true)
  })

  it('ignores the retired collapsedWorkspaces key a previous build persisted', async () => {
    await saveSidebarUI({ collapsedRepos: [], collapsedWorkspaces: ['ws3', 'ws1'] })
    await hydrateSidebar()
    const state = useSidebarStore.getState() as unknown as Record<string, unknown>
    expect(state.collapsedWorkspaces).toBeUndefined()
    expect(useSidebarStore.getState().collapsedChatRows.size).toBe(0)
  })

  // REGRESSION (restyle v2): the old build's project-row fold persisted this
  // set and project-visibility gated a folded project's streams on it; the
  // restyled sidebar has no writer, so a project it folded rendered blank
  // whenever it was not active. The key is retired like collapsedRepos.
  it('ignores the retired collapsedProjects key a previous build persisted', async () => {
    await saveSidebarUI({
      collapsedRepos: [],
      collapsedWorkspaces: [],
      collapsedProjects: ['p2', 'p3'],
      collapsedChatRows: ['f1'],
    })
    await hydrateSidebar()
    const state = useSidebarStore.getState() as unknown as Record<string, unknown>
    expect(state.collapsedProjects).toBeUndefined()
    expect(useSidebarStore.getState().collapsedChatRows.has('f1')).toBe(true)
  })

  it('restores collapsedChatRows from IDB', async () => {
    await saveSidebarUI({ collapsedRepos: [], collapsedChatRows: ['f1', 'c7'] })
    await hydrateSidebar()
    const { collapsedChatRows } = useSidebarStore.getState()
    expect(collapsedChatRows.has('f1')).toBe(true)
    expect(collapsedChatRows.has('c7')).toBe(true)
  })

  it('replays a record written before the Chats panel was collapsible as "all open"', async () => {
    useSidebarStore.setState({ collapsedChatRows: new Set(['stale']) })
    await saveSidebarUI({ collapsedRepos: ['crowbar'], collapsedWorkspaces: [] })
    await hydrateSidebar()
    expect(useSidebarStore.getState().collapsedChatRows.size).toBe(0)
  })
})
