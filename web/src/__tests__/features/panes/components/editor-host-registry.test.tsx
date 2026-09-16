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
      path: `/${id}.ts`,
      name: `${id}.ts`,
      content: '',
      savedContent: '',
      isDirty: false,
      isVirtual: false,
      tokens: [],
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
  // Regression: live-reported the SAME blank-pane symptom EditorHostRegistry
  // was built to eliminate, reappearing with no repro steps. Root cause: an
  // earlier version of EditorHostSlot returned null (unmounting its portaled
  // EditorPane) whenever the registry's published entry was momentarily
  // undefined — which happens on EVERY dependency change of PaneContainer's
  // own registration effect (its cleanup clears the entry, its new body sets
  // it) — not only when the pane's last editor tab actually closes. This
  // simulates that exact clear-then-set sequence directly against the
  // registry (the same two calls PaneContainer's effect makes back to back)
  // and asserts the portaled EditorPane survives it without unmounting.
  it('does not unmount the portaled EditorPane across a clear-then-reset of its registry entry', async () => {
    seedEditorTab(ROOT_PANE_ID, 'tab-a')
    const node = createPortalTarget()

    render(createElement(EditorHostRegistry))

    act(() => {
      setEditorPortalEntry(ROOT_PANE_ID, {
        node,
        activeEditorBufferId: 'tab-a',
        isPreview: false,
        isActiveSurface: true,
      })
    })

    const before = await screen.findByTestId('editor-marker-tab-a')
    expect(editorMountCount.current).toBe(1)

    // The exact sequence PaneContainer's registration effect runs when ANY
    // of its dependencies change (a tab switch, a focus change, ...): its
    // cleanup fires first (clearing the entry), then its new body
    // immediately re-publishes a fresh one. Forced into SEPARATE `act`s (one
    // commit each) rather than one — batched into a single commit, React
    // only ever renders the end state and never actually exercises the gap
    // this regression is about; production's two effect-phase writes are not
    // guaranteed to land in the same commit either.
    act(() => {
      clearEditorPortalEntry(ROOT_PANE_ID)
    })
    act(() => {
      setEditorPortalEntry(ROOT_PANE_ID, {
        node,
        activeEditorBufferId: 'tab-a',
        isPreview: false,
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
        isPreview: false,
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
