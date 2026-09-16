import { createElement, useEffect } from 'react'
import { act, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'

vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/editor/stores/buffer-session-persistence', () => ({
  saveSessionToStore: vi.fn(),
  clearQueuedWorkspaceSessionSave: vi.fn(),
}))

/**
 * A pane stood in by a marker that counts its own EFFECT-mounts — the same
 * instrument `pane-container.test.tsx` uses for the identical question, and
 * the only one that can tell "re-rendered" from "torn down and rebuilt". A
 * render counter would not: React re-renders a surviving component too.
 */
const { mounts } = vi.hoisted(() => ({ mounts: { current: new Map<string, number>() } }))
vi.mock('@/features/panes/components/pane-container', () => ({
  PaneContainer: ({ pane, showing }: { pane: { id: string }; showing?: boolean }) => {
    useEffect(() => {
      mounts.current.set(pane.id, (mounts.current.get(pane.id) ?? 0) + 1)
    }, [pane.id])
    return createElement('div', {
      'data-testid': `pane-${pane.id}`,
      'data-showing': String(showing ?? true),
    })
  },
}))

// Neither is relevant to which view renders, and both drag in real chrome.
vi.mock('@/features/window/stores/ui-state-store', () => ({
  useUIState: (sel: (s: { isBottomPaneVisible: boolean }) => unknown) =>
    sel({ isBottomPaneVisible: false }),
}))

import { SplitViewRoot } from '@/features/panes/components/split-view-root'

const mountsOf = (paneId: string) => mounts.current.get(paneId) ?? 0

beforeEach(() => {
  mounts.current = new Map()
  resetWindowPaneStoreForTests()
})
afterEach(() => {
  resetWindowPaneStoreForTests()
})

/** Two views, each one pane: `ROOT_PANE_ID` and a second one. Returns the
 *  second view/pane id (they are the same — `addPane` names a view after the
 *  pane it mints). */
function openSecondView(): string {
  const { paneActions } = windowPaneStore.getState()
  paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', null)
  const second = paneActions.addPane()!
  paneActions.setPaneChat(second, 'chat-2', null)
  return second
}

describe('SplitViewRoot — only the active view occupies the content area', () => {
  it('renders every open view, and hides all but the showing one', async () => {
    const second = openSecondView()
    await act(async () => {
      render(createElement(SplitViewRoot))
    })

    const showing = screen.getByTestId(`pane-${second}`)
    const parked = screen.getByTestId(`pane-${ROOT_PANE_ID}`)
    expect(showing.getAttribute('data-showing')).toBe('true')
    expect(parked.getAttribute('data-showing')).toBe('false')

    // The parked view's wrapper is what removes it from the layout — and it
    // is `inert`, so nothing off screen is focusable or clickable.
    const parkedRoot = document.querySelector(`[data-view-root="${ROOT_PANE_ID}"]`)!
    expect(parkedRoot.getAttribute('style')).toContain('display: none')
    expect(parkedRoot.hasAttribute('inert')).toBe(true)
    expect(document.querySelector(`[data-view-root="${second}"]`)!.hasAttribute('inert')).toBe(
      false,
    )
  })

  /**
   * THE REGRESSION. Rendering the showing view in one place and the parked
   * ones in another keeps every view "mounted" in the loosest sense and is
   * still wrong: switching moves a subtree between two parents, which React
   * can only do by unmounting it and mounting a new one. Measured live, that
   * destroyed a parked view's xterm — and because xterm re-initialisation is
   * gated on `isVisible`, it never came back.
   */
  it('switching views does NOT remount either view — parking is not a teardown', async () => {
    const second = openSecondView()
    await act(async () => {
      render(createElement(SplitViewRoot))
    })
    expect(mountsOf(ROOT_PANE_ID)).toBe(1)
    expect(mountsOf(second)).toBe(1)

    await act(async () => {
      windowPaneStore.getState().paneActions.activateView(ROOT_PANE_ID)
    })
    expect(screen.getByTestId(`pane-${ROOT_PANE_ID}`).getAttribute('data-showing')).toBe('true')
    expect(screen.getByTestId(`pane-${second}`).getAttribute('data-showing')).toBe('false')

    // ...and back again, so neither direction of the swap is a teardown.
    await act(async () => {
      windowPaneStore.getState().paneActions.activateView(second)
    })

    expect(mountsOf(ROOT_PANE_ID)).toBe(1)
    expect(mountsOf(second)).toBe(1)
  })

  it('a view keeps its DOM node across a switch', async () => {
    openSecondView()
    await act(async () => {
      render(createElement(SplitViewRoot))
    })
    const before = screen.getByTestId(`pane-${ROOT_PANE_ID}`)

    await act(async () => {
      windowPaneStore.getState().paneActions.activateView(ROOT_PANE_ID)
    })

    // Identity, not just presence: a remount would hand back a different node
    // even though the query still finds one.
    expect(screen.getByTestId(`pane-${ROOT_PANE_ID}`)).toBe(before)
  })

  it('a newly opened view is the only one showing', async () => {
    openSecondView()

    await act(async () => {
      render(createElement(SplitViewRoot))
    })

    let third = ''
    await act(async () => {
      third = windowPaneStore.getState().paneActions.addPane()!
      windowPaneStore.getState().paneActions.setPaneChat(third, 'chat-3', null)
    })

    const showing = [...document.querySelectorAll('[data-view-root]')].filter(
      (e) => !e.hasAttribute('data-parked-view'),
    )
    expect(showing).toHaveLength(1)
    expect(showing[0].getAttribute('data-view-root')).toBe(third)
  })

  it('closing the showing view reveals the one behind it, without remounting it', async () => {
    const second = openSecondView()
    await act(async () => {
      render(createElement(SplitViewRoot))
    })
    expect(mountsOf(ROOT_PANE_ID)).toBe(1)

    await act(async () => {
      windowPaneStore.getState().paneActions.closeView(second)
    })

    expect(screen.queryByTestId(`pane-${second}`)).toBeNull()
    expect(screen.getByTestId(`pane-${ROOT_PANE_ID}`).getAttribute('data-showing')).toBe('true')
    // The revealed view was parked, not rebuilt — it is the same instance it
    // has been since it was opened.
    expect(mountsOf(ROOT_PANE_ID)).toBe(1)
  })
})
