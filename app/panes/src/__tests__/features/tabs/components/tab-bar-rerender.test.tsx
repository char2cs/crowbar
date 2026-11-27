import { act, fireEvent, render, screen } from '@testing-library/react'
import { createElement, Profiler } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import type { EditorContent } from '@/features/panes/types/pane-content'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { resetWindowPaneStoreForTests } from '@/__tests__/__fixtures__/window-pane-store'

const iconRenderCounts = new Map<string, number>()
vi.mock('@/features/file-explorer/components/file-explorer-icon', () => ({
  FileExplorerIcon: ({ fileName }: { fileName: string }) => {
    iconRenderCounts.set(fileName, (iconRenderCounts.get(fileName) ?? 0) + 1)
    return createElement('span', { 'data-testid': 'file-icon' })
  },
}))

vi.mock('@/components/ui/sidebar', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/components/ui/sidebar')>()
  return {
    ...actual,
    useSidebar: () => ({ open: true, toggleSidebar: () => {} }),
  }
})

vi.mock('@/features/tabs/components/tab-context-menu', () => ({
  default: ({ isOpen, buffer }: { isOpen: boolean; buffer: { id: string } | null }) =>
    isOpen && buffer
      ? createElement('div', { 'data-testid': 'ctx-menu', 'data-buffer-id': buffer.id })
      : null,
}))

import TabBar from '@/features/tabs/components/tab-bar'

function makeEditorBuffer(i: number, overrides: Partial<EditorContent> = {}): EditorContent {
  return {
    id: `buf-${i}`,
    type: 'editor',
    authority: 'workspace',
    path: `/project/file-${i}.ts`,
    name: `file-${i}.ts`,
    isDirty: false,
    isVirtual: false,
    isPinned: false,
    isPreview: false,
    workspaceId: 'w1',
    ...overrides,
  }
}

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

    const before = new Map(iconRenderCounts)
    expect(before.get('file-2.ts')).toBeGreaterThan(0)

    act(() => {
      windowPaneStore.setState((s) => {
        s.buffers = s.buffers.map((b) =>
          b.id === 'buf-2' && b.type === 'editor' ? { ...b, isDirty: true } : b,
        )
        return s
      })
    })

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

    const tab2 = screen.getByRole('tab', { name: /file-2\.ts/ }).closest('.group\\/tab')
    const closeBtn = tab2?.querySelectorAll('button')[1] as HTMLElement
    act(() => {
      fireEvent.click(closeBtn)
    })

    expect(windowPaneStore.getState().buffers.some((b) => b.id === 'buf-2')).toBe(false)
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.editorTabIds).not.toContain('buf-2')
  })
})

describe('TabBar strip-level render isolation', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('does not re-render the strip when a buffer write leaves rendered fields unchanged', () => {
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

    act(() => {
      windowPaneStore.setState((s) => {
        s.buffers = s.buffers.map((b) =>
          b.id === 'buf-2' && b.type === 'editor' ? { ...b, languageOverride: 'plaintext' } : b,
        )
        return s
      })
    })

    expect(commits).toBe(baseline)
  })
})

describe('TabBar drag preview', () => {
  it('lifts the dragged tab onto the control surface, whatever the pane behind it is', () => {
    const store = setupStore([makeEditorBuffer(0), makeEditorBuffer(1)])
    act(() => {
      renderTabBar(store)
    })
    const pointer = { pointerId: 1, isPrimary: true, button: 0, buttons: 1 }
    act(() => {
      fireEvent.pointerDown(screen.getAllByRole('tab')[0], { ...pointer, clientX: 10, clientY: 10 })
      fireEvent.pointerMove(document, { ...pointer, clientX: 40, clientY: 10 })
    })
    const preview = document.querySelector('.tab-drag-preview')!
    expect(preview.textContent).toContain('file-0.ts')
    expect(preview.classList.contains('control-surface')).toBe(true)
    expect(preview.closest('[data-drag-overlay]')).not.toBeNull()
    act(() => {
      fireEvent.pointerUp(document, { ...pointer, buttons: 0, clientX: 40, clientY: 10 })
    })
  })
})
