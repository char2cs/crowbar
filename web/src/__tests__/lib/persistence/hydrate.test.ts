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
  hydratePreferences,
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
import { createLeaf } from '@/features/panes/utils/pane-layout'
import { saveSidebarUI } from '@/lib/persistence/sidebar-ui'
import { saveWorkspaceHierarchy } from '@/lib/persistence/workspace-hierarchy'
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
      [ROOT_PANE_ID]: { id: ROOT_PANE_ID, type: 'group', bufferIds: [], activeBufferId: null },
    },
    rootLayout: createLeaf(ROOT_PANE_ID),
    bottomLayout: createLeaf('bottom-pane'),
    activePaneId: ROOT_PANE_ID,
    mostRecentActivePaneIds: [ROOT_PANE_ID],
    buffers: [],
    sidebarWidth: 240,
    rightSidebarWidth: 280,
    updatedAt: Date.now(),
  }
  const prefs: UIPreferences = {
    theme: 'dark',
    fontSize: 14,
    fontFamily: 'JetBrains Mono',
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

  it('restores panes/buffers from the one window-session row', async () => {
    const db = await getDB()
    const rightPaneId = 'pane-right'
    await db.put('workspace-layout', {
      workspaceId: WINDOW_SESSION_ID,
      panes: {
        [ROOT_PANE_ID]: {
          id: ROOT_PANE_ID,
          type: 'group',
          chatId: null,
          runnerId: null,
          editorTabIds: ['buf-1'],
          activeEditorTabId: 'buf-1',
          editorOpen: true,
        },
        [rightPaneId]: {
          id: rightPaneId,
          type: 'group',
          chatId: null,
          runnerId: null,
          editorTabIds: [],
          activeEditorTabId: null,
          editorOpen: false,
        },
      },
      rootLayout: createLeaf(ROOT_PANE_ID),
      bottomLayout: createLeaf('bottom-pane'),
      activePaneId: rightPaneId,
      mostRecentActivePaneIds: [rightPaneId, ROOT_PANE_ID],
      buffers: [
        {
          id: 'buf-1',
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
        },
      ],
      sidebarWidth: 240,
      rightSidebarWidth: 280,
      updatedAt: Date.now(),
    })

    await hydrateWindowPaneLayout()

    const state = windowPaneStore.getState()
    expect(state.activePaneId).toBe(rightPaneId)
    expect(Object.keys(state.panes).sort()).toEqual([ROOT_PANE_ID, rightPaneId].sort())
    expect(state.buffers).toHaveLength(1)
    expect(state.buffers[0]).toMatchObject({ id: 'buf-1', content: 'saved' })
  })
})

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
      isActive: true,
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
        },
      },
      rootLayout: createLeaf(ROOT_PANE_ID),
      bottomLayout: createLeaf('bottom-pane'),
      activePaneId: ROOT_PANE_ID,
      mostRecentActivePaneIds: [ROOT_PANE_ID],
      buffers,
      sidebarWidth: 240,
      rightSidebarWidth: 280,
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
      isActive: true,
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

describe('hydratePreferences', () => {
  beforeEach(async () => {
    resetDB()
    globalThis.indexedDB = new IDBFactory()
  })

  it('returns null when no prefs are stored', async () => {
    const prefs = await hydratePreferences()
    expect(prefs).toBeNull()
  })

  it('returns stored preferences', async () => {
    const { prefs } = await seedDB('ws-test')
    const result = await hydratePreferences()
    expect(result?.theme).toBe(prefs.theme)
  })
})

describe('hydrateSidebar', () => {
  beforeEach(async () => {
    resetDB()
    globalThis.indexedDB = new IDBFactory()
    useSidebarStore.setState({
      repos: HYDRATE_TEST_REPOS.map((r) => ({ ...r, workspaces: [...r.workspaces] })),
      collapsedRepos: new Set<string>(),
      collapsedWorkspaces: new Set<string>(),
      activeTab: 'workspaces',
    })
  })

  it('does nothing when IDB is empty', async () => {
    await hydrateSidebar()
    expect(useSidebarStore.getState().collapsedRepos.size).toBe(0)
  })

  it('restores collapsedRepos from IDB', async () => {
    await saveSidebarUI({ collapsedRepos: ['crowbar', 'quiver-core'], collapsedWorkspaces: [] })
    await hydrateSidebar()
    const { collapsedRepos } = useSidebarStore.getState()
    expect(collapsedRepos.has('crowbar')).toBe(true)
    expect(collapsedRepos.has('quiver-core')).toBe(true)
  })

  it('overlays parentId values from IDB onto repos', async () => {
    await saveWorkspaceHierarchy('crowbar', [
      { wsId: 'ws3', parentId: 'ws-develop' },
      { wsId: 'ws1', parentId: 'ws3' },
    ])
    await hydrateSidebar()
    const repo = useSidebarStore.getState().repos.find((r) => r.id === 'crowbar')!
    expect(repo.workspaces.find((w) => w.id === 'ws3')?.parentId).toBe('ws-develop')
    expect(repo.workspaces.find((w) => w.id === 'ws1')?.parentId).toBe('ws3')
  })

  it('clears parentId for workspaces not in hierarchy entries', async () => {
    await saveWorkspaceHierarchy('crowbar', [{ wsId: 'ws1' }])
    await hydrateSidebar()
    const repo = useSidebarStore.getState().repos.find((r) => r.id === 'crowbar')!
    expect(repo.workspaces.find((w) => w.id === 'ws1')?.parentId).toBeUndefined()
  })

  it('restores collapsedWorkspaces from IDB', async () => {
    await saveSidebarUI({ collapsedRepos: [], collapsedWorkspaces: ['ws3', 'ws1'] })
    await hydrateSidebar()
    const { collapsedWorkspaces } = useSidebarStore.getState()
    expect(collapsedWorkspaces.has('ws3')).toBe(true)
    expect(collapsedWorkspaces.has('ws1')).toBe(true)
  })

  it('restores collapsedProjects from IDB', async () => {
    await saveSidebarUI({
      collapsedRepos: [],
      collapsedWorkspaces: [],
      collapsedProjects: ['p2', 'p3'],
    })
    await hydrateSidebar()
    const { collapsedProjects } = useSidebarStore.getState()
    expect(collapsedProjects.has('p2')).toBe(true)
    expect(collapsedProjects.has('p3')).toBe(true)
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

  it('replays a record written before projects were collapsible as "all open"', async () => {
    useSidebarStore.setState({ collapsedProjects: new Set(['stale']) })
    const db = await getDB()
    // A record with no collapsedProjects key at all — either a build that
    // predates collapsible projects, or one that wrote the old inverted
    // `expandedProjects`. Both replay as "nothing collapsed", which is the
    // product default: a fresh install shows every project open.
    await db.put(
      'sidebar-ui',
      { collapsedRepos: ['crowbar'], expandedProjects: ['p9'], updatedAt: Date.now() } as never,
      'global',
    )
    await hydrateSidebar()
    expect(useSidebarStore.getState().collapsedProjects.size).toBe(0)
  })
})
