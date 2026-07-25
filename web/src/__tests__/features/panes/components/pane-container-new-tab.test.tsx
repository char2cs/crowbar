import { createElement, useMemo } from 'react'
import { act, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import {
  WorkspaceStoreContext,
  useWorkspaceStoreContext,
} from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'

// NewTabView pulls in the router, sidebar store, project store and keymaps to
// render its own content (task 6's concern, covered by new-tab-view.test.tsx).
// This test is about HOSTING — does a `newTab` buffer resolve to NewTabView in
// the pane's render switch — so stub it to a passive marker, mirroring how
// diff-pane/terminal-pane are stubbed in the sibling suspense/agent-chat tests.
// Mocked via the SAME '@/...' id pane-container's relative './new-tab-view'
// import resolves to, so the mock intercepts it.
vi.mock('@/features/panes/components/new-tab-view', () => ({
  // I7: every action on this surface must target the pane it is DRAWN in, not
  // whatever pane happens to be globally active — which requires PaneContainer
  // to actually hand it a paneId. Rendering the received prop (rather than
  // ignoring it) lets both hosting sites below assert that happened.
  NewTabView: ({ paneId }: { paneId?: string }) =>
    createElement('div', { 'data-testid': 'new-tab-marker', 'data-pane-id': paneId ?? '' }),
}))

// TabBar drags in SidebarProvider/dnd-kit machinery irrelevant to this test,
// so it's stubbed — but the stub still reads the pane's REAL bufferIds/buffers
// off the same workspace store (not a canned prop), rendering one `role="tab"`
// per open buffer. That lets the two tests below tell "the newTab buffer
// resolved through the switch case" apart from "the pane has no active buffer
// at all": both render the identical NewTabView marker in the content area,
// but only the former has a buffer sitting in the pane's tab strip. If the
// `newTab` switch case ever regresses by excluding the buffer from the pane's
// render list, this stub goes tab-less exactly like the true empty-pane case,
// and the assertion on tab count catches it where the marker alone would not.
vi.mock('@/features/tabs/components/tab-bar', () => {
  // Named (not anonymous) so react-hooks/rules-of-hooks recognises this as a
  // component rather than an arbitrary function that happens to call hooks.
  function MockTabBar({ paneId }: { paneId?: string }) {
    // Select the raw, referentially-stable store slices only — deriving the
    // per-buffer summary in the selector itself would return a fresh array
    // every call and defeat useSyncExternalStore's bail-out (infinite loop).
    const bufferIds = useWorkspaceStoreContext((s) => (paneId ? s.panes[paneId]?.bufferIds : null))
    const buffers = useWorkspaceStoreContext((s) => s.buffers)
    const bufferSummaries = useMemo(() => {
      if (!bufferIds) return []
      return bufferIds
        .map((id) => buffers.find((b) => b.id === id))
        .filter((b): b is NonNullable<typeof b> => Boolean(b))
        .map((b) => ({ id: b.id, type: b.type }))
    }, [bufferIds, buffers])
    return createElement(
      'div',
      { role: 'tablist' },
      bufferSummaries.map((b) =>
        createElement(
          'div',
          { key: b.id, role: 'tab', 'data-testid': `tab-${b.type}` },
          b.type === 'newTab' ? 'New Tab' : b.type,
        ),
      ),
    )
  }
  return { default: MockTabBar }
})

import { PaneContainer } from '@/features/panes/components/pane-container'

function PaneHost() {
  const pane = useWorkspaceStoreContext((s) => s.panes[ROOT_PANE_ID])
  if (!pane) return null
  return createElement(PaneContainer, { pane })
}

async function renderPane(store: ReturnType<typeof createWorkspaceStore>) {
  await act(async () => {
    render(createElement(WorkspaceStoreContext.Provider, { value: store }, createElement(PaneHost)))
  })
}

describe('PaneContainer New Tab hosting', () => {
  it('renders NewTabView for the active newTab buffer', async () => {
    const store = createWorkspaceStore('w1')
    // A fresh workspace store's root pane starts with no tabs; openNewTab is the
    // real action that spawns and activates the New Tab buffer (same path a
    // pane emptying out or ⌘T takes) — using it keeps this test's setup exactly
    // as production reaches this state, not a hand-rolled buffer literal.
    store.getState().bufferActions.openNewTab(ROOT_PANE_ID)

    await renderPane(store)

    expect(await screen.findByTestId('new-tab-marker')).toBeInTheDocument()
    // Proves the `newTab` switch case fired (the buffer resolved through the
    // pane's render list), not just that the fallback rendered the same
    // marker: the buffer also shows up as a real tab in the strip.
    expect(screen.getByRole('tab')).toHaveTextContent('New Tab')
    // I7: the `newTab` switch case must hand NewTabView ITS OWN pane id, not
    // render it prop-less (which left every action targeting whatever pane
    // happened to be globally active instead of this one).
    expect(screen.getByTestId('new-tab-marker')).toHaveAttribute('data-pane-id', ROOT_PANE_ID)
  })

  it('falls back to NewTabView when the pane has no active buffer at all', async () => {
    // A brand-new store's root pane starts with bufferIds: [] and
    // activeBufferId: null — the "stranded pane" state the brief's fallback
    // guards. Under the New Tab rules this should be unreachable in practice
    // (a pane always spawns/keeps a New Tab), but the fallback must still
    // resolve to a usable surface rather than a blank rectangle.
    const store = createWorkspaceStore('w1')

    await renderPane(store)

    expect(await screen.findByTestId('new-tab-marker')).toBeInTheDocument()
    // Distinguishes this from the case above: here the pane genuinely holds no
    // buffers, so the tab strip must be empty — same marker, different cause.
    expect(screen.queryByRole('tab')).not.toBeInTheDocument()
    // I7: the fallback site needs the same paneId wiring as the switch case.
    expect(screen.getByTestId('new-tab-marker')).toHaveAttribute('data-pane-id', ROOT_PANE_ID)
  })
})
