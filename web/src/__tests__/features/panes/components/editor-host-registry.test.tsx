import { createElement, useEffect } from 'react'
import { act, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import {
  clearEditorPortalEntry,
  setEditorPortalEntry,
} from '@/features/panes/lib/editor-portal-registry'

const { editorMountCount } = vi.hoisted(() => ({ editorMountCount: { current: 0 } }))
vi.mock('@/features/panes/components/editor-pane', () => ({
  EditorPane: ({ bufferId }: { bufferId: string }) => {
    useEffect(() => {
      editorMountCount.current += 1
    }, [])
    return createElement('div', { 'data-testid': `editor-marker-${bufferId}` })
  },
}))

import { EditorHostRegistry } from '@/features/panes/components/editor-host-registry'

// createPortal's target must be attached to document.body for RTL's `screen`
// (which queries document.body) to find its content — tracked here so each
// test's target node is removed afterward without touching RTL's own
// render container.
let portalTargets: HTMLElement[] = []
function createPortalTarget(): HTMLDivElement {
  const node = document.createElement('div')
  document.body.appendChild(node)
  portalTargets.push(node)
  return node
}

beforeEach(() => {
  resetWindowPaneStoreForTests()
  editorMountCount.current = 0
  portalTargets = []
})
afterEach(() => {
  clearEditorPortalEntry(ROOT_PANE_ID)
  for (const node of portalTargets) node.remove()
})

function seedEditorTab(paneId: string, id: string) {
  windowPaneStore.setState((state) => {
    state.buffers.push({
      id,
      type: 'editor',
      authority: 'workspace',
      path: `/${id}.ts`,
      name: `${id}.ts`,
      isDirty: false,
      isVirtual: false,
      isPinned: false,
      isPreview: false,
      workspaceId: 'w1',
    })
    return state
  })
  windowPaneStore.getState().paneActions.addEditorTabToPane(paneId, {
    id,
    type: 'editor',
    name: `${id}.ts`,
    workspaceId: 'w1',
  })
}

describe('EditorHostRegistry / EditorHostSlot', () => {
  // PaneContainer's registration effect clears then re-sets the entry on every dependency
  // change; the portaled EditorPane must survive that gap, unmounting only when the pane's
  // last editor tab closes.
  it('does not unmount the portaled EditorPane across a clear-then-reset of its registry entry', async () => {
    seedEditorTab(ROOT_PANE_ID, 'tab-a')
    const node = createPortalTarget()

    render(createElement(EditorHostRegistry))

    act(() => {
      setEditorPortalEntry(ROOT_PANE_ID, {
        node,
        activeEditorBufferId: 'tab-a',
        isActiveSurface: true,
      })
    })

    const before = await screen.findByTestId('editor-marker-tab-a')
    expect(editorMountCount.current).toBe(1)

    // Separate `act`s (one commit each): batched, React renders only the end state and never
    // exercises the gap, and production's two writes aren't guaranteed one commit either.
    act(() => {
      clearEditorPortalEntry(ROOT_PANE_ID)
    })
    act(() => {
      setEditorPortalEntry(ROOT_PANE_ID, {
        node,
        activeEditorBufferId: 'tab-a',
        isActiveSurface: true,
      })
    })

    expect(screen.getByTestId('editor-marker-tab-a')).toBe(before) // same node — never unmounted
    expect(editorMountCount.current).toBe(1) // never remounted, ever
  })

  it('does unmount once the pane genuinely loses its last editor tab', async () => {
    seedEditorTab(ROOT_PANE_ID, 'tab-a')
    const node = createPortalTarget()

    render(createElement(EditorHostRegistry))
    act(() => {
      setEditorPortalEntry(ROOT_PANE_ID, {
        node,
        activeEditorBufferId: 'tab-a',
        isActiveSurface: true,
      })
    })
    await screen.findByTestId('editor-marker-tab-a')

    act(() => {
      windowPaneStore.getState().paneActions.removeEditorTabFromPane(ROOT_PANE_ID, 'tab-a')
    })

    expect(screen.queryByTestId('editor-marker-tab-a')).not.toBeInTheDocument()
  })
})
