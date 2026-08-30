import { createElement, Fragment, useEffect } from 'react'
import { act, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  WorkspaceStoreContext,
  useWorkspaceStoreContext,
} from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { setActiveWorkspaceStoreRef } from '@/features/workspace/stores/workspace-store-ref'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'

// Fix round 1 (Task 18 review): a mount counter, not just a DOM-identity
// check, for the "does toggling pane.chatId remount a live terminal" test
// below — a real PTY-backed surface, stood in for by a marker that counts
// its own EFFECT-mount (not render) exactly the way TerminalPane's real
// PTY-attach effect would only fire once per genuine mount.
const { terminalMountCount } = vi.hoisted(() => ({ terminalMountCount: { current: 0 } }))
vi.mock('@/features/panes/components/terminal-pane', () => ({
  TerminalPane: ({ sessionId, bufferId }: { sessionId?: string; bufferId: string }) => {
    useEffect(() => {
      terminalMountCount.current += 1
    }, [])
    return createElement('div', {
      'data-testid': `terminal-marker-${bufferId}`,
      'data-session-id': sessionId ?? '',
    })
  },
}))

// Task 31: PaneContainer migrated off the old bufferIds/activeBufferId/
// previewBufferId shape onto pane.editorTabIds/activeEditorTabId and the
// pane's own chatId/runnerId — chat is a first-class pane field now, no
// longer an 'agentChat' buffer competing with editor tabs for "active".
//
// AgentChatPane/EditorPane/NewTabView are heavy surfaces (PTY attach, Monaco/
// Plate, router+sidebar+keymaps) with their own dedicated tests — this file
// is about HOSTING: does PaneContainer mount the right sibling(s) for a given
// pane shape, and does it call the renamed pane-slice actions. Stub each to a
// passive marker recording the props PaneContainer threads through, mirroring
// the pattern already used by pane-container-suspense.test.tsx.
vi.mock('@/features/agent/components/agent-chat-pane', () => ({
  AgentChatPane: ({
    chatId,
    runnerId,
    wsId,
    bufferId,
    isActivePane,
    isVisible,
  }: {
    chatId: string
    runnerId: string
    wsId: string
    bufferId: string
    isActivePane: boolean
    isVisible: boolean
  }) =>
    createElement('div', {
      'data-testid': `chat-${chatId}`,
      'data-runner-id': runnerId,
      'data-ws-id': wsId,
      'data-buffer-id': bufferId,
      'data-active-pane': String(isActivePane),
      'data-visible': String(isVisible),
    }),
}))

vi.mock('@/features/panes/components/editor-pane', () => ({
  EditorPane: ({
    bufferId,
    isPreview,
    isActiveSurface,
  }: {
    bufferId: string
    isPreview: boolean
    isActiveSurface: boolean
  }) =>
    createElement('div', {
      'data-testid': `editor-marker-${bufferId}`,
      'data-preview': String(isPreview),
      'data-active-surface': String(isActiveSurface),
    }),
}))

vi.mock('@/features/panes/components/new-tab-view', () => ({
  NewTabView: ({ paneId }: { paneId?: string }) =>
    createElement('div', { 'data-testid': 'new-tab-marker', 'data-pane-id': paneId ?? '' }),
}))

// TabBar drags in SidebarProvider/dnd-kit machinery irrelevant to hosting.
vi.mock('@/features/tabs/components/tab-bar', () => ({
  default: () => null,
}))

// Exposes the real onDrop handler PaneContainer wires up (handleSplitDrop) via
// plain buttons, so a drag-drop test can fire it without simulating HTML5 DnD
// through the real overlay's own zone-geometry math. `centerDropPayload` is
// mutated per-test (vi.mock factories are hoisted and only ever instantiated
// once) so the center-zone button can carry a test-specific source pane/tab.
const { centerDropPayload } = vi.hoisted(() => ({
  centerDropPayload: { current: { bufferId: 'existing-tab', paneId: 'phantom-source-pane' } },
}))
vi.mock('@/features/panes/components/split-drop-overlay', () => ({
  SplitDropOverlay: ({ onDrop }: { onDrop: (zone: string, e: unknown) => void }) =>
    createElement(
      Fragment,
      null,
      createElement('button', {
        type: 'button',
        'data-testid': 'split-drop-trigger-right',
        onClick: () =>
          onDrop('right', {
            dataTransfer: {
              getData: (type: string) =>
                type === 'application/tab-data'
                  ? JSON.stringify({ bufferId: 'existing-tab', paneId: 'phantom-source-pane' })
                  : '',
            },
          }),
      }),
      createElement('button', {
        type: 'button',
        'data-testid': 'split-drop-trigger-center',
        onClick: () =>
          onDrop('center', {
            dataTransfer: {
              getData: (type: string) =>
                type === 'application/tab-data' ? JSON.stringify(centerDropPayload.current) : '',
            },
          }),
      }),
    ),
}))

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

/** Seeds one editor-tab buffer directly (bypassing buffer-slice's own
 *  openContent/addBufferToPane, which is unmigrated and calls a pane action
 *  that no longer exists) and registers it on the pane via the real,
 *  currently-functional addEditorTabToPane — the same pattern
 *  buffer-slice.test.ts/pane-slice.test.ts already use to seed tabs. */
function seedEditorTab(
  store: ReturnType<typeof createWorkspaceStore>,
  paneId: string,
  id: string,
  overrides: { isPreview?: boolean } = {},
) {
  store.setState((state) => {
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
      isPreview: overrides.isPreview ?? false,
    })
    return state
  })
  store.getState().paneActions.addEditorTabToPane(paneId, {
    id,
    type: 'editor',
    name: `${id}.ts`,
  })
}

/** A terminal tab — a buffer whose editor-view rendering is a real PTY
 *  attachment in production (stubbed above to a mount-counting marker). Used
 *  by the "does pane.chatId toggling remount the editor view" regression:
 *  an editor tab alone proves DOM survival, but a terminal is what actually
 *  loses live state (its PTY) on a spurious remount. */
function seedTerminalTab(
  store: ReturnType<typeof createWorkspaceStore>,
  paneId: string,
  id: string,
) {
  store.setState((state) => {
    state.buffers.push({
      id,
      type: 'terminal',
      name: `term-${id}`,
      sessionId: `session-${id}`,
      isPinned: false,
    })
    return state
  })
  store.getState().paneActions.addEditorTabToPane(paneId, {
    id,
    type: 'terminal',
    name: `term-${id}`,
  })
}

describe('PaneContainer — chat/editor-view hosting', () => {
  afterEach(() => {
    setActiveWorkspaceStoreRef(null)
  })

  it('renders the chat, not NewTabView, when the pane has a chat and zero editor tabs', async () => {
    const store = createWorkspaceStore('w1')
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')

    await renderPane(store)

    const chat = await screen.findByTestId('chat-chat-1')
    expect(chat).toHaveAttribute('data-runner-id', 'runner-1')
    expect(chat).toHaveAttribute('data-ws-id', 'w1')
    expect(chat).toHaveAttribute('data-visible', 'true')
    // The editor region (and its NewTabView fallback) stays MOUNTED per spec
    // §7.2 — "renders the chat, not NewTabView" means not SHOWING, not "isn't
    // in the DOM at all". See the dedicated "keeps both mounted" test below
    // for the mount-vs-visibility distinction spelled out explicitly.
    expect(screen.getByTestId('new-tab-marker')).not.toBeVisible()
    expect(chat).toBeVisible()
  })

  it('renders the active tab content and no chat surface when the pane has editor tabs and no chat', async () => {
    const store = createWorkspaceStore('w1')
    seedEditorTab(store, ROOT_PANE_ID, 'tab-a')

    await renderPane(store)

    expect(await screen.findByTestId('editor-marker-tab-a')).toBeInTheDocument()
    expect(screen.queryByTestId(/^chat-/)).not.toBeInTheDocument()
  })

  it('falls back to NewTabView when the pane has no chat and no editor tabs', async () => {
    const store = createWorkspaceStore('w1')

    await renderPane(store)

    expect(await screen.findByTestId('new-tab-marker')).toHaveAttribute(
      'data-pane-id',
      ROOT_PANE_ID,
    )
  })

  it('renders both the chat and the editor tab when the pane holds both', async () => {
    const store = createWorkspaceStore('w1')
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    // addEditorTabToPane sets pane.editorOpen = true as a side effect — the
    // split-toggle state that gates whether the editor view shows alongside
    // an existing chat (spec §7.1/§7.2).
    seedEditorTab(store, ROOT_PANE_ID, 'tab-a')

    await renderPane(store)

    expect(await screen.findByTestId('chat-chat-1')).toBeInTheDocument()
    expect(await screen.findByTestId('editor-marker-tab-a')).toBeInTheDocument()
  })

  it('a pane with both a chat and editor tabs, split toggled off, keeps both mounted and only hides the editor view', async () => {
    const store = createWorkspaceStore('w1')
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    seedEditorTab(store, ROOT_PANE_ID, 'tab-a')
    // addEditorTabToPane sets editorOpen = true as a side effect (see the test
    // above) — force the split back off so this test can assert the "chat-only"
    // state without losing the editor tab.
    store.setState((state) => {
      const pane = state.panes[ROOT_PANE_ID]
      if (pane) pane.editorOpen = false
      return state
    })

    await renderPane(store)

    const chat = await screen.findByTestId('chat-chat-1')
    const editorMarker = await screen.findByTestId('editor-marker-tab-a')
    // Both are MOUNTED regardless of which is showing (spec §7.2: "Both
    // surfaces stay mounted"). The editor's marker is present in the DOM...
    expect(editorMarker).toBeInTheDocument()
    // ...but its content-area ancestor carries the native `hidden` attribute
    // (display:none via the UA stylesheet — not a Tailwind class, so this
    // assertion needs no compiled CSS to be meaningful), while the chat's does
    // not.
    expect(editorMarker.closest('[hidden]')).not.toBeNull()
    expect(chat.closest('[hidden]')).toBeNull()
  })

  it('keeps the chat mounted (same DOM node) across an editor-tab activation', async () => {
    const store = createWorkspaceStore('w1')
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    seedEditorTab(store, ROOT_PANE_ID, 'tab-a')
    seedEditorTab(store, ROOT_PANE_ID, 'tab-b')

    await renderPane(store)

    const chatBefore = await screen.findByTestId('chat-chat-1')

    await act(async () => {
      store.getState().paneActions.activateEditorTabInPane(ROOT_PANE_ID, 'tab-a')
    })

    const chatAfter = screen.getByTestId('chat-chat-1')
    // Same DOM node — switching which editor tab is active never remounts
    // the chat region, which sits outside editorTabIds/activeEditorTabId
    // entirely (it doesn't compete for "which one is active").
    expect(chatAfter).toBe(chatBefore)
  })

  it("reads the active tab's preview styling from its own isPreview field", async () => {
    const store = createWorkspaceStore('w1')
    seedEditorTab(store, ROOT_PANE_ID, 'tab-preview', { isPreview: true })

    await renderPane(store)

    expect(await screen.findByTestId('editor-marker-tab-preview')).toHaveAttribute(
      'data-preview',
      'true',
    )
  })

  it('does not read preview styling off a removed pane-level id', async () => {
    const store = createWorkspaceStore('w1')
    seedEditorTab(store, ROOT_PANE_ID, 'tab-plain', { isPreview: false })

    await renderPane(store)

    expect(await screen.findByTestId('editor-marker-tab-plain')).toHaveAttribute(
      'data-preview',
      'false',
    )
    // PaneGroup no longer carries previewBufferId at all (Task 1) — nothing
    // in the pane state this test seeded could make the tab preview except
    // its own isPreview field, which was set to false above.
    expect(
      (store.getState().panes[ROOT_PANE_ID] as unknown as Record<string, unknown>).previewBufferId,
    ).toBe(undefined)
  })

  it('a split-zone drop of an existing tab calls the renamed editor-tab actions, not the removed buffer actions', async () => {
    const store = createWorkspaceStore('w1')
    await renderPane(store)

    // PaneContainer reads `workspaceStore.getState().paneActions.<method>` fresh
    // at call time (not a destructured closure captured at render), so patching
    // the method in place on the store's own (already-created) actions object is
    // observed by the real call site. Avoids vi.spyOn here: its auto-restore at
    // suite teardown trips over this plain object's property descriptor.
    const moveCalls: unknown[][] = []
    const activateCalls: unknown[][] = []
    const paneActions = store.getState().paneActions
    const originalMove = paneActions.moveEditorTabToPane.bind(paneActions)
    const originalActivate = paneActions.activateEditorTabInPane.bind(paneActions)
    paneActions.moveEditorTabToPane = (...args: Parameters<typeof originalMove>) => {
      moveCalls.push(args)
      return originalMove(...args)
    }
    paneActions.activateEditorTabInPane = (...args: Parameters<typeof originalActivate>) => {
      activateCalls.push(args)
      return originalActivate(...args)
    }

    const trigger = await screen.findByTestId('split-drop-trigger-right')
    await act(async () => {
      trigger.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })

    expect(moveCalls).toHaveLength(1)
    expect(moveCalls[0][0]).toBe('existing-tab')
    expect(moveCalls[0][1]).toBe('phantom-source-pane')
    expect(typeof moveCalls[0][2]).toBe('string')
    expect(activateCalls).toHaveLength(1)
    expect(activateCalls[0][1]).toBe('existing-tab')
    // The old buffer-vocabulary actions this migration retired do not exist
    // on the actions object at all — a regression reintroducing them (or
    // pane-container calling them) would fail loudly, not silently no-op.
    const actions = store.getState().paneActions as unknown as Record<string, unknown>
    expect(actions.moveBufferToPane).toBeUndefined()
    expect(actions.activatePaneBuffer).toBeUndefined()
    expect(actions.addBufferToPane).toBeUndefined()
  })

  it('a center-zone drop of an existing tab from another pane moves it in via the migrated pane-drop-actions helpers', async () => {
    const store = createWorkspaceStore('w1')
    // The center zone routes through pane-drop-actions.ts's
    // moveBufferToPaneDropTarget/ensureBufferInPaneDropTarget, which read the
    // GLOBAL active-workspace-store ref (a separate registry from the
    // WorkspaceStoreContext.Provider renderPane uses below) — the same setup
    // pane-drop-actions.test.ts already needs for these same two helpers.
    setActiveWorkspaceStoreRef(store)

    const sourcePaneId = store.getState().paneActions.splitPane(ROOT_PANE_ID, 'horizontal')
    if (!sourcePaneId) throw new Error('splitPane did not create a source pane')
    store.setState((state) => {
      state.buffers.push({
        id: 'moved-tab',
        type: 'editor',
        path: '/moved.ts',
        name: 'moved.ts',
        content: '',
        savedContent: '',
        isDirty: false,
        isVirtual: false,
        tokens: [],
        isPinned: false,
        isPreview: false,
      })
      return state
    })
    store.getState().paneActions.addEditorTabToPane(sourcePaneId, {
      id: 'moved-tab',
      type: 'editor',
      name: 'moved.ts',
    })
    centerDropPayload.current = { bufferId: 'moved-tab', paneId: sourcePaneId }

    await renderPane(store)

    const trigger = await screen.findByTestId('split-drop-trigger-center')
    await act(async () => {
      trigger.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })

    // Before this fix round, pane-drop-actions.ts's own helpers still called
    // the removed moveBufferToPane/addBufferToPane/activatePaneBuffer — this
    // proves the tab actually lands in ROOT_PANE_ID, not just that clicking
    // through didn't throw.
    expect(store.getState().panes[ROOT_PANE_ID]?.editorTabIds).toContain('moved-tab')
    expect(store.getState().panes[sourcePaneId]?.editorTabIds ?? []).not.toContain('moved-tab')
  })
})

// Task 18: usePaneViewPresentation wired into the chat/editor arrangement,
// replacing Task 31's placeholder sequential stack. usePaneViewPresentation's
// own geometry math (side-by-side vs. stacked vs. tabs thresholds) has its own
// dedicated unit tests in use-chat-presentation.test.ts against a plain
// { clientWidth, clientHeight } double — no real layout needed. This file is
// about the WIRING: does PaneContainer arrange the two real DOM regions
// (divider or not, hidden or not, row or column) the way that presentation
// says to, and do both regions genuinely survive every presentation change.
describe('PaneContainer — chat/editor-view arrangement (spec §7.2)', () => {
  afterEach(() => {
    setActiveWorkspaceStoreRef(null)
  })

  /** jsdom reports 0 for every element's clientWidth/clientHeight (no layout
   *  engine) — usePaneViewPresentation treats an unmeasured 0x0 pane as
   *  side-by-side (see its own "flash" comment), so that is the size this
   *  suite gets for free without overriding anything. */
  it('an unmeasured pane with the split on defaults to side by side: a divider, and the editor is not hidden', async () => {
    const store = createWorkspaceStore('w1')
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    seedEditorTab(store, ROOT_PANE_ID, 'tab-a') // addEditorTabToPane sets editorOpen = true

    await renderPane(store)

    const chat = await screen.findByTestId('chat-chat-1')
    const editorMarker = await screen.findByTestId('editor-marker-tab-a')
    expect(chat.closest('[hidden]')).toBeNull()
    expect(editorMarker.closest('[hidden]')).toBeNull()
    expect(screen.getByRole('separator')).toHaveAttribute('aria-orientation', 'vertical')
  })

  it('with the split off, there is no divider — tabs, not a cramped split', async () => {
    const store = createWorkspaceStore('w1')
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    seedEditorTab(store, ROOT_PANE_ID, 'tab-a')
    store.setState((state) => {
      const pane = state.panes[ROOT_PANE_ID]
      if (pane) pane.editorOpen = false
      return state
    })

    await renderPane(store)

    await screen.findByTestId('chat-chat-1')
    expect(screen.queryByRole('separator')).not.toBeInTheDocument()
  })

  /** Forces a genuinely portrait pane (narrow, tall) past jsdom's normal 0x0
   *  by overriding the one measurement usePaneViewPresentation actually reads
   *  — clientWidth/clientHeight — for the duration of the test. Everything
   *  else about jsdom (no real layout) is untouched. */
  async function withPaneBox<T>(width: number, height: number, run: () => Promise<T>): Promise<T> {
    const widthDesc = Object.getOwnPropertyDescriptor(Element.prototype, 'clientWidth')
    const heightDesc = Object.getOwnPropertyDescriptor(Element.prototype, 'clientHeight')
    Object.defineProperty(Element.prototype, 'clientWidth', {
      configurable: true,
      get: () => width,
    })
    Object.defineProperty(Element.prototype, 'clientHeight', {
      configurable: true,
      get: () => height,
    })
    try {
      return await run()
    } finally {
      if (widthDesc) Object.defineProperty(Element.prototype, 'clientWidth', widthDesc)
      if (heightDesc) Object.defineProperty(Element.prototype, 'clientHeight', heightDesc)
    }
  }

  it('a portrait pane stacks the two views vertically, with a horizontal-orientation divider', async () => {
    await withPaneBox(500, 1200, async () => {
      const store = createWorkspaceStore('w1')
      store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
      seedEditorTab(store, ROOT_PANE_ID, 'tab-a')

      await renderPane(store)

      const chat = await screen.findByTestId('chat-chat-1')
      const editorMarker = await screen.findByTestId('editor-marker-tab-a')
      expect(chat.closest('[hidden]')).toBeNull()
      expect(editorMarker.closest('[hidden]')).toBeNull()
      // PaneSash's aria-orientation is the OPPOSITE of its layout direction
      // (same convention agent-chat-pane-split.test.tsx relies on for its own
      // terminal split) — 'horizontal' here means the sash itself divides
      // top from bottom, i.e. the views are stacked in a column.
      expect(screen.getByRole('separator')).toHaveAttribute('aria-orientation', 'horizontal')
    })
  })

  it('a landscape pane presents the two views side by side, with a vertical-orientation divider', async () => {
    await withPaneBox(1600, 500, async () => {
      const store = createWorkspaceStore('w1')
      store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
      seedEditorTab(store, ROOT_PANE_ID, 'tab-a')

      await renderPane(store)

      await screen.findByTestId('chat-chat-1')
      expect(screen.getByRole('separator')).toHaveAttribute('aria-orientation', 'vertical')
    })
  })

  it('too small on both axes falls back to tabs even with the split on', async () => {
    await withPaneBox(300, 200, async () => {
      const store = createWorkspaceStore('w1')
      store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
      seedEditorTab(store, ROOT_PANE_ID, 'tab-a')

      await renderPane(store)

      const editorMarker = await screen.findByTestId('editor-marker-tab-a')
      expect(editorMarker.closest('[hidden]')).not.toBeNull()
      expect(screen.queryByRole('separator')).not.toBeInTheDocument()
    })
  })

  it('never unmounts either view across an editorOpen toggle — same DOM nodes throughout', async () => {
    const store = createWorkspaceStore('w1')
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    seedEditorTab(store, ROOT_PANE_ID, 'tab-a')

    await renderPane(store)

    const chatBefore = await screen.findByTestId('chat-chat-1')
    const editorBefore = await screen.findByTestId('editor-marker-tab-a')

    await act(async () => {
      store.setState((state) => {
        const pane = state.panes[ROOT_PANE_ID]
        if (pane) pane.editorOpen = false
        return state
      })
    })

    expect(screen.getByTestId('chat-chat-1')).toBe(chatBefore)
    expect(screen.getByTestId('editor-marker-tab-a')).toBe(editorBefore)

    await act(async () => {
      store.setState((state) => {
        const pane = state.panes[ROOT_PANE_ID]
        if (pane) pane.editorOpen = true
        return state
      })
    })

    expect(screen.getByTestId('chat-chat-1')).toBe(chatBefore)
    expect(screen.getByTestId('editor-marker-tab-a')).toBe(editorBefore)
  })

  // Fix round 1: the Critical bug a review caught in the first version of
  // this wiring — a `pane.chatId ? <A/> : <B/>` top-level branch reindexed
  // the editor view's own DOM position, so React unmounted/remounted it (and
  // everything live inside it, e.g. a terminal's PTY) every time
  // `pane.chatId` toggled. Reachable in production today via `⌘N`
  // (use-pane-keyboard.ts's `setPaneChat` on the active pane whatever it
  // already holds) and chat-removal.ts's `setPaneChat(paneId, null, null)`
  // the other way — NOT a hypothetical pane.chatId is not yet set.
  it('does not remount the editor view — including a live terminal — when pane.chatId toggles on and off', async () => {
    terminalMountCount.current = 0
    const store = createWorkspaceStore('w1')
    seedTerminalTab(store, ROOT_PANE_ID, 'term-a')

    await renderPane(store)

    const terminalBefore = await screen.findByTestId('terminal-marker-term-a')
    expect(terminalMountCount.current).toBe(1)

    // pane.chatId: null -> set. This is the exact transition the bug lost —
    // the editor view (and the terminal buffer inside it) used to live
    // directly under data-pane-content; once a chat exists it gets
    // re-parented one level deeper, under viewsContainerRef, alongside the
    // new chat view and sash.
    await act(async () => {
      store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    })

    await screen.findByTestId('chat-chat-1')
    expect(screen.getByTestId('terminal-marker-term-a')).toBe(terminalBefore)
    expect(terminalMountCount.current).toBe(1) // still exactly one mount, ever

    // And back: set -> null.
    await act(async () => {
      store.getState().paneActions.setPaneChat(ROOT_PANE_ID, null, null)
    })

    expect(screen.queryByTestId('chat-chat-1')).not.toBeInTheDocument()
    expect(screen.getByTestId('terminal-marker-term-a')).toBe(terminalBefore)
    expect(terminalMountCount.current).toBe(1)
  })
})
