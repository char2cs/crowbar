import { act, fireEvent, render, screen } from '@testing-library/react'
import { createElement, Profiler } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import type { EditorContent } from '@/features/panes/types/pane-content'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { windowPaneStore, resetWindowPaneStoreForTests } from '@/features/panes/stores/window-pane-store'

// ── Per-item render probe ────────────────────────────────────────────
// Each TabBarItem renders FileExplorerIcon INLINE (it is not independently
// memoized), so the mock's invocation count for a given fileName is exactly
// the number of times that item's TabBarItem re-rendered. Keyed by fileName
// (== buffer.name), which the dirty-toggle test deliberately keeps stable.
const iconRenderCounts = new Map<string, number>()
vi.mock('@/features/file-explorer/components/file-explorer-icon', () => ({
  FileExplorerIcon: ({ fileName }: { fileName: string }) => {
    iconRenderCounts.set(fileName, (iconRenderCounts.get(fileName) ?? 0) + 1)
    return createElement('span', { 'data-testid': 'file-icon' })
  },
}))

// TabBar reaches useSidebar() only for the fallback sidebar-reopen toggle,
// irrelevant to tab rendering. Stub it so the suite needn't stand up a
// SidebarProvider; `open: true` keeps the reopen toggle out of the tree.
vi.mock('@/components/ui/sidebar', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/components/ui/sidebar')>()
  return {
    ...actual,
    useSidebar: () => ({ open: true, toggleSidebar: () => {} }),
  }
})

// Stub the context menu down to a marker that echoes the buffer it was opened
// for — lets the delegation sweep assert the right buffer reached
// handleContextMenu without the real menu's portal/positioning machinery.
vi.mock('@/features/tabs/components/tab-context-menu', () => ({
  default: ({ isOpen, buffer }: { isOpen: boolean; buffer: { id: string } | null }) =>
    isOpen && buffer
      ? createElement('div', { 'data-testid': 'ctx-menu', 'data-buffer-id': buffer.id })
      : null,
}))

// Imported AFTER the mocks above (vi.mock is hoisted regardless, but keep the
// intent explicit).
import TabBar from '@/features/tabs/components/tab-bar'

function makeEditorBuffer(i: number, overrides: Partial<EditorContent> = {}): EditorContent {
  return {
    id: `buf-${i}`,
    type: 'editor',
    path: `/project/file-${i}.ts`,
    name: `file-${i}.ts`,
    content: '',
    savedContent: '',
    isDirty: false,
    isVirtual: false,
    isPinned: false,
    isPreview: false,
    isActive: i === 0,
    tokens: [],
    workspaceId: 'w1',
    ...overrides,
  }
}

// Task 26: panes/buffers moved off the per-workspace store onto the
// window-level singleton — seed `windowPaneStore`, not the per-workspace
// store `createWorkspaceStore` still returns (kept for WorkspaceStoreContext).
function setupStore(buffers: EditorContent[]) {
  const store = createWorkspaceStore('w1')
  resetWindowPaneStoreForTests()
  windowPaneStore.setState((s) => {
    s.buffers = buffers
    s.panes[ROOT_PANE_ID] = {
      ...s.panes[ROOT_PANE_ID],
      editorTabIds: buffers.map((b) => b.id),
      activeEditorTabId: buffers[0]?.id ?? null,
    }
    return s
  })
  return store
}

function renderTabBar(store: ReturnType<typeof createWorkspaceStore>) {
  return render(
    createElement(
      WorkspaceStoreContext.Provider,
      { value: store },
      createElement(TabBar, { paneId: ROOT_PANE_ID }),
    ),
  )
}

describe('TabBar render isolation', () => {
  beforeEach(() => {
    iconRenderCounts.clear()
  })

  afterEach(() => {
    vi.clearAllMocks()
  })

  it('re-renders only the changed tab when one buffer toggles dirty', () => {
    const buffers = Array.from({ length: 5 }, (_, i) => makeEditorBuffer(i))
    const store = setupStore(buffers)

    act(() => {
      renderTabBar(store)
    })

    // Baseline captured AFTER mount effects settle (edge-detection layout
    // effect + auto-scroll effect each may trigger one extra render pass).
    const before = new Map(iconRenderCounts)
    expect(before.get('file-2.ts')).toBeGreaterThan(0)

    // Flip an INACTIVE buffer's dirty flag while REUSING every other buffer's
    // object reference — only buf-2's identity changes, exactly as an immer
    // copy-on-write content flush would leave the untouched tabs shared.
    act(() => {
      windowPaneStore.setState((s) => {
        s.buffers = s.buffers.map((b) =>
          b.id === 'buf-2' && b.type === 'editor' ? { ...b, isDirty: true } : b,
        )
        return s
      })
    })

    // Exactly the mutated tab re-renders; the other four are memoized out.
    expect(iconRenderCounts.get('file-2.ts')).toBe((before.get('file-2.ts') ?? 0) + 1)
    for (const i of [0, 1, 3, 4]) {
      expect(iconRenderCounts.get(`file-${i}.ts`)).toBe(before.get(`file-${i}.ts`))
    }
  })
})

describe('TabBar handler delegation', () => {
  beforeEach(() => {
    iconRenderCounts.clear()
  })

  afterEach(() => {
    vi.clearAllMocks()
  })

  it('routes a click to the correct buffer', () => {
    const buffers = Array.from({ length: 5 }, (_, i) => makeEditorBuffer(i))
    const store = setupStore(buffers)
    act(() => {
      renderTabBar(store)
    })

    const tabs = screen.getAllByRole('tab')
    act(() => {
      fireEvent.click(tabs[3])
    })

    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.activeEditorTabId).toBe('buf-3')
  })

  it('routes a double-click to the correct buffer (promotes that preview tab)', () => {
    const buffers = Array.from({ length: 5 }, (_, i) =>
      makeEditorBuffer(i, i === 1 ? { isPreview: true } : {}),
    )
    const store = setupStore(buffers)
    act(() => {
      renderTabBar(store)
    })

    expect(
      (windowPaneStore.getState().buffers.find((b) => b.id === 'buf-1') as EditorContent).isPreview,
    ).toBe(true)

    const tabs = screen.getAllByRole('tab')
    act(() => {
      fireEvent.doubleClick(tabs[1])
    })

    expect(
      (windowPaneStore.getState().buffers.find((b) => b.id === 'buf-1') as EditorContent).isPreview,
    ).toBe(false)
  })

  it('routes a context-menu to the correct buffer', () => {
    const buffers = Array.from({ length: 5 }, (_, i) => makeEditorBuffer(i))
    const store = setupStore(buffers)
    act(() => {
      renderTabBar(store)
    })

    const tabs = screen.getAllByRole('tab')
    act(() => {
      fireEvent.contextMenu(tabs[2])
    })

    expect(screen.getByTestId('ctx-menu').getAttribute('data-buffer-id')).toBe('buf-2')
  })

  it('routes keyboard ArrowRight to the correct index (activates the next tab)', () => {
    const buffers = Array.from({ length: 5 }, (_, i) => makeEditorBuffer(i))
    const store = setupStore(buffers)
    act(() => {
      renderTabBar(store)
    })

    const tabs = screen.getAllByRole('tab')
    act(() => {
      fireEvent.keyDown(tabs[0], { key: 'ArrowRight' })
    })

    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.activeEditorTabId).toBe('buf-1')
  })

  it('routes the close affordance to the correct buffer', () => {
    const buffers = Array.from({ length: 5 }, (_, i) => makeEditorBuffer(i))
    const store = setupStore(buffers)
    act(() => {
      renderTabBar(store)
    })

    // The close Button is the 2nd button inside each tab's group container.
    const tab2 = screen.getByRole('tab', { name: /file-2\.ts/ }).closest('.group\\/tab')
    const closeBtn = tab2?.querySelectorAll('button')[1] as HTMLElement
    act(() => {
      fireEvent.click(closeBtn)
    })

    expect(windowPaneStore.getState().buffers.some((b) => b.id === 'buf-2')).toBe(false)
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.editorTabIds).not.toContain('buf-2')
  })
})

// TabBarItem's memo (asserted above) isolates WHICH tab repaints. This closes
// TabBar's OWN re-render: a content flush that leaves every rendered field
// unchanged must not re-render the strip at all (no sort, no remap). The
// icon-count probe can't see this — TabBarItem's memo already bails on
// unchanged content — so we count commits of the TabBar subtree with a
// Profiler instead: a bailed-out subtree produces no commit, hence no onRender.
describe('TabBar strip-level render isolation', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('does not re-render the strip when a buffer content flush leaves rendered fields unchanged', () => {
    const buffers = Array.from({ length: 5 }, (_, i) => makeEditorBuffer(i))
    const store = setupStore(buffers)

    let commits = 0
    act(() => {
      render(
        createElement(
          WorkspaceStoreContext.Provider,
          { value: store },
          createElement(
            Profiler,
            {
              id: 'tab-bar',
              onRender: () => {
                commits += 1
              },
            },
            createElement(TabBar, { paneId: ROOT_PANE_ID }),
          ),
        ),
      )
    })

    const baseline = commits
    expect(baseline).toBeGreaterThan(0)

    // Change ONLY `content` on one buffer (a non-rendered field) while reusing
    // every other buffer's reference — exactly an immer content flush. No
    // rendered field moves, so the strip must not commit again.
    act(() => {
      windowPaneStore.setState((s) => {
        s.buffers = s.buffers.map((b) =>
          b.id === 'buf-2' && b.type === 'editor' ? { ...b, content: 'typed a char' } : b,
        )
        return s
      })
    })

    expect(commits).toBe(baseline)
  })
})
