import { createElement, Fragment, useEffect } from 'react'
import { useStore } from 'zustand'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { setActiveWorkspaceStoreRef } from '@/features/workspace/stores/workspace-store-ref'
import { windowPaneStore, resetWindowPaneStoreForTests } from '@/features/panes/stores/window-pane-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { ROOT_PANE_POSITION, type PanePosition } from '@/features/panes/types/pane'
import { buildPaneContentStyle } from '@/features/panes/utils/pane-border'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import type { InternalDropZone } from '@/features/tabs/utils/internal-tab-drag'

// Task F: lets a test stand in an edge zone (left/right/top/bottom) for a
// file-tree drop's resolved target without faking `document.elementsFromPoint`
// geometry — `resolveDropTarget`'s real geometry math already has its own
// dedicated coverage (pane-drop-zones.test.ts); this file only needs to prove
// what PaneContainer DOES with whatever zone it resolves to. Falls through to
// the real implementation whenever a test hasn't set an override, so every
// other test in this file (none of which drag files) is unaffected.
const { resolveDropTargetOverride } = vi.hoisted(() => ({
  resolveDropTargetOverride: {
    current: null as { paneId: string | null; zone: InternalDropZone } | null,
  },
}))
vi.mock('@/features/tabs/utils/internal-tab-drag', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('@/features/tabs/utils/internal-tab-drag')>()
  return {
    ...actual,
    resolveDropTarget: (point: { x: number; y: number }) =>
      resolveDropTargetOverride.current ?? actual.resolveDropTarget(point),
  }
})

// Task 9 (sidebar restyle recovery batch 2): lets a test force the sidebar
// closed without standing up a real SidebarProvider (which drags in
// useMediaQuery/matchMedia) — pane-container.tsx reads only
// `useSidebarOptional()?.open`, so overriding that alone is enough to put a
// pane against a REAL window edge (isWindowEdge's collapsed-sidebar branch),
// not just the common sidebar-shielded case every other test in this file
// exercises by default.
const { sidebarOpenOverride } = vi.hoisted(() => ({ sidebarOpenOverride: { current: true } }))
vi.mock('@/components/ui/sidebar', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/components/ui/sidebar')>()
  return {
    ...actual,
    useSidebarOptional: (): ReturnType<typeof actual.useSidebarOptional> =>
      ({ open: sidebarOpenOverride.current }) as ReturnType<typeof actual.useSidebarOptional>,
  }
})

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
// Renders a plain marker (rather than null) so Task 9's tests can find WHERE
// in the tree the identity row lands — specifically, whether it is nested
// inside the same painted/rounded box as the content, or an unstyled sibling
// of it.
vi.mock('@/features/tabs/components/tab-bar', () => ({
  default: () => createElement('div', { 'data-testid': 'tab-bar-marker' }),
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

function PaneHost({ position }: { position?: PanePosition }) {
  // Task 26: panes are window-level now — read off windowPaneStore, not the
  // per-workspace WorkspaceStoreContext.
  const pane = useStore(windowPaneStore, (s) => s.panes[ROOT_PANE_ID])
  if (!pane) return null
  return createElement(PaneContainer, { pane, position })
}

// `position` defaults to ROOT_PANE_POSITION (PaneContainer's own default) for
// every existing caller; Task 9's window-edge/interior-pane tests pass one
// explicitly to control which of the pane's own edges are real window edges.
async function renderPane(store: ReturnType<typeof createWorkspaceStore>, position?: PanePosition) {
  await act(async () => {
    render(
      createElement(
        WorkspaceStoreContext.Provider,
        { value: store },
        createElement(PaneHost, { position }),
      ),
    )
  })
}

/** Seeds one editor-tab buffer directly (bypassing buffer-slice's own
 *  openContent/addBufferToPane, which is unmigrated and calls a pane action
 *  that no longer exists) and registers it on the pane via the real,
 *  currently-functional addEditorTabToPane — the same pattern
 *  buffer-slice.test.ts/pane-slice.test.ts already use to seed tabs. */
function seedEditorTab(
  _store: ReturnType<typeof createWorkspaceStore>,
  paneId: string,
  id: string,
  overrides: { isPreview?: boolean } = {},
) {
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
      isPreview: overrides.isPreview ?? false,
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

/** A terminal tab — a buffer whose editor-view rendering is a real PTY
 *  attachment in production (stubbed above to a mount-counting marker). Used
 *  by the "does pane.chatId toggling remount the editor view" regression:
 *  an editor tab alone proves DOM survival, but a terminal is what actually
 *  loses live state (its PTY) on a spurious remount. */
function seedTerminalTab(
  _store: ReturnType<typeof createWorkspaceStore>,
  paneId: string,
  id: string,
) {
  windowPaneStore.setState((state) => {
    state.buffers.push({
      id,
      type: 'terminal',
      name: `term-${id}`,
      sessionId: `session-${id}`,
      isPinned: false,
      workspaceId: 'w1',
    })
    return state
  })
  windowPaneStore.getState().paneActions.addEditorTabToPane(paneId, {
    id,
    type: 'terminal',
    name: `term-${id}`,
    workspaceId: 'w1',
  })
}

// Task 26: panes/buffers are a window-level singleton now, never destroyed —
// reset it before every test in this file so one test's seeded panes/tabs
// never leak into the next (each test used to get this for free from its own
// isolated createWorkspaceStore() instance).
beforeEach(() => {
  resetWindowPaneStoreForTests()
})

describe('PaneContainer — chat/editor-view hosting', () => {
  afterEach(() => {
    setActiveWorkspaceStoreRef(null)
  })

  it('renders the chat, not NewTabView, when the pane has a chat and zero editor tabs', async () => {
    const store = createWorkspaceStore('w1')
    windowPaneStore.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')

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
    windowPaneStore.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
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
    windowPaneStore.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    seedEditorTab(store, ROOT_PANE_ID, 'tab-a')
    // addEditorTabToPane sets editorOpen = true as a side effect (see the test
    // above) — force the split back off so this test can assert the "chat-only"
    // state without losing the editor tab.
    windowPaneStore.setState((state) => {
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
    windowPaneStore.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    seedEditorTab(store, ROOT_PANE_ID, 'tab-a')
    seedEditorTab(store, ROOT_PANE_ID, 'tab-b')

    await renderPane(store)

    const chatBefore = await screen.findByTestId('chat-chat-1')

    await act(async () => {
      windowPaneStore.getState().paneActions.activateEditorTabInPane(ROOT_PANE_ID, 'tab-a')
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
      (windowPaneStore.getState().panes[ROOT_PANE_ID] as unknown as Record<string, unknown>).previewBufferId,
    ).toBe(undefined)
  })

  it('a split-zone drop of an existing tab calls the renamed editor-tab actions, not the removed buffer actions', async () => {
    const store = createWorkspaceStore('w1')
    await renderPane(store)

    // PaneContainer reads `windowPaneStore.getState().paneActions.<method>` fresh
    // at call time (not a destructured closure captured at render), so swapping
    // in a wrapped actions object before the click is observed by the real call
    // site. Immer's autoFreeze deep-freezes the store's `paneActions` object the
    // first time ANY `set()` runs against `windowPaneStore` (which the shared
    // singleton's own reset in `beforeEach` already triggers) — patching a method
    // in place on that frozen object throws, so replace the whole object via
    // `setState` instead of reassigning one of its properties.
    const moveCalls: unknown[][] = []
    const activateCalls: unknown[][] = []
    const paneActions = windowPaneStore.getState().paneActions
    const originalMove = paneActions.moveEditorTabToPane.bind(paneActions)
    const originalActivate = paneActions.activateEditorTabInPane.bind(paneActions)
    windowPaneStore.setState({
      paneActions: {
        ...paneActions,
        moveEditorTabToPane: (...args: Parameters<typeof originalMove>) => {
          moveCalls.push(args)
          return originalMove(...args)
        },
        activateEditorTabInPane: (...args: Parameters<typeof originalActivate>) => {
          activateCalls.push(args)
          return originalActivate(...args)
        },
      },
    })

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
    const actions = windowPaneStore.getState().paneActions as unknown as Record<string, unknown>
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

    const sourcePaneId = windowPaneStore.getState().paneActions.splitPane(ROOT_PANE_ID, 'horizontal')
    if (!sourcePaneId) throw new Error('splitPane did not create a source pane')
    windowPaneStore.setState((state) => {
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
        workspaceId: 'w1',
      })
      return state
    })
    windowPaneStore.getState().paneActions.addEditorTabToPane(sourcePaneId, {
      id: 'moved-tab',
      type: 'editor',
      name: 'moved.ts',
      workspaceId: 'w1',
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
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.editorTabIds).toContain('moved-tab')
    expect(windowPaneStore.getState().panes[sourcePaneId]?.editorTabIds ?? []).not.toContain('moved-tab')
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
    windowPaneStore.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
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
    windowPaneStore.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    seedEditorTab(store, ROOT_PANE_ID, 'tab-a')
    windowPaneStore.setState((state) => {
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
      windowPaneStore.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
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
      windowPaneStore.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
      seedEditorTab(store, ROOT_PANE_ID, 'tab-a')

      await renderPane(store)

      await screen.findByTestId('chat-chat-1')
      expect(screen.getByRole('separator')).toHaveAttribute('aria-orientation', 'vertical')
    })
  })

  it('too small on both axes falls back to tabs even with the split on', async () => {
    await withPaneBox(300, 200, async () => {
      const store = createWorkspaceStore('w1')
      windowPaneStore.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
      seedEditorTab(store, ROOT_PANE_ID, 'tab-a')

      await renderPane(store)

      const editorMarker = await screen.findByTestId('editor-marker-tab-a')
      expect(editorMarker.closest('[hidden]')).not.toBeNull()
      expect(screen.queryByRole('separator')).not.toBeInTheDocument()
    })
  })

  it('never unmounts either view across an editorOpen toggle — same DOM nodes throughout', async () => {
    const store = createWorkspaceStore('w1')
    windowPaneStore.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    seedEditorTab(store, ROOT_PANE_ID, 'tab-a')

    await renderPane(store)

    const chatBefore = await screen.findByTestId('chat-chat-1')
    const editorBefore = await screen.findByTestId('editor-marker-tab-a')

    await act(async () => {
      windowPaneStore.setState((state) => {
        const pane = state.panes[ROOT_PANE_ID]
        if (pane) pane.editorOpen = false
        return state
      })
    })

    expect(screen.getByTestId('chat-chat-1')).toBe(chatBefore)
    expect(screen.getByTestId('editor-marker-tab-a')).toBe(editorBefore)

    await act(async () => {
      windowPaneStore.setState((state) => {
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
  // already holds) — NOT a hypothetical pane.chatId is not yet set.
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
      windowPaneStore.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    })

    await screen.findByTestId('chat-chat-1')
    expect(screen.getByTestId('terminal-marker-term-a')).toBe(terminalBefore)
    expect(terminalMountCount.current).toBe(1) // still exactly one mount, ever

    // And back: set -> null.
    await act(async () => {
      windowPaneStore.getState().paneActions.setPaneChat(ROOT_PANE_ID, null, null)
    })

    expect(screen.queryByTestId('chat-chat-1')).not.toBeInTheDocument()
    expect(screen.getByTestId('terminal-marker-term-a')).toBe(terminalBefore)
    expect(terminalMountCount.current).toBe(1)
  })
})

// Task 22: the sidebar's drag arm (`useSidebarDrag`) hit-tests a pane by
// reading `PANE_DROP_ATTR` (`data-pane-drop`) straight off the DOM — every
// rendered pane has to carry ITS OWN id on that attribute for a chat/row
// dropped on it to resolve to the right pane and zone (spec §8.1). This
// replaces the old dwell-to-remove overlay, which published one bare
// `data-pane-drop=""` flag for the WHOLE content region (ide-shell.tsx) and
// painted its "release to remove" veil through `editor-removal-overlay.tsx` —
// both deleted with this task, not migrated.
describe('PaneContainer — pane drop target (spec §8.1, Task 22)', () => {
  it('publishes its own pane id on PANE_DROP_ATTR, not a bare presence flag', async () => {
    const store = createWorkspaceStore('w1')
    await renderPane(store)

    const container = document.querySelector('[data-pane-container]')!
    expect(container.getAttribute('data-pane-drop')).toBe(ROOT_PANE_ID)
  })

  it('carries no trace of the deleted dwell-to-remove overlay', async () => {
    const store = createWorkspaceStore('w1')
    await renderPane(store)

    expect(document.querySelector('[data-pane-removal]')).toBeNull()
  })
})

// Task F: a file dragged from the Files panel and dropped on a pane's EDGE
// zone used to fall into openFileTreeDropInPane's `getPaneSplitDropOptions`/
// `splitPane` branch — copied from the legitimate row/chat-drag split
// mechanic (handleSplitDrop below, spec §8.1) onto a path where it is
// explicitly forbidden. Spec §6.3: "Clicking a file opens it in the editor
// view of the focused pane, never in a pane of its own." Spec §7.2: "Nothing
// lands in a pane of its own; everything lands in the editor view." A file
// drop must always resolve to the EXISTING pane it was dropped on, regardless
// of zone.
describe('PaneContainer — file-tree drop never creates a pane of its own (spec §6.3/§7.2, Task F)', () => {
  afterEach(() => {
    setActiveWorkspaceStoreRef(null)
    delete (window as unknown as { __fileDragData?: unknown }).__fileDragData
    useFileSystemStore.setState({ handleFileOpen: null })
    resolveDropTargetOverride.current = null
  })

  it("dropping a file on a pane's EDGE zone opens it as a tab in that SAME pane — no new pane is created", async () => {
    const store = createWorkspaceStore('w1')
    seedEditorTab(store, ROOT_PANE_ID, 'tab-a')

    // Stands in for the real handler wired by use-workspace-effects.ts
    // (openFileContent → bufferActions.openContent) minus the network fetch —
    // it exercises the REAL openContent/addEditorTabToPane pane-routing logic,
    // which is exactly what a regression in openFileTreeDropInPane's target
    // pane selection would misroute.
    useFileSystemStore.setState({
      handleFileOpen: async (path: string) => {
        windowPaneStore.getState().bufferActions.openContent({
          type: 'editor',
          path,
          name: path.split('/').pop() ?? path,
          content: '',
          workspaceId: 'w1',
        })
      },
    })

    await renderPane(store)

    const paneIdsBefore = Object.keys(windowPaneStore.getState().panes).sort()
    const tabCountBefore = windowPaneStore.getState().panes[ROOT_PANE_ID]?.editorTabIds.length ?? 0

    // The resolved drop target names ROOT_PANE_ID but at zone 'left' — an
    // EDGE zone, the exact zone that used to route through splitPane() for a
    // file drop (Task F's root cause). A file drop must ignore this zone
    // entirely and still land in ROOT_PANE_ID.
    resolveDropTargetOverride.current = { paneId: ROOT_PANE_ID, zone: 'left' }
    window.__fileDragData = { type: 'file', path: '/dropped.ts', name: 'dropped.ts', isDir: false }

    const container = document.querySelector('[data-pane-container]')!
    await act(async () => {
      fireEvent.mouseUp(container, { clientX: 5, clientY: 200 })
      // openFileTreeDropInPane's body runs after an `await handleFileOpen(...)`
      // — flush the microtask queue so its post-await work (addExistingTabToPane
      // /activateEditorTabInPane) has committed before assertions run.
      await Promise.resolve()
      await Promise.resolve()
    })

    // No new pane was created — the pane tree shape is byte-for-byte unchanged.
    expect(Object.keys(windowPaneStore.getState().panes).sort()).toEqual(paneIdsBefore)

    // The file landed as a NEW tab in the SAME pane it was dropped on, not a
    // freshly split one.
    const openedBuffer = windowPaneStore.getState().buffers.find((b) => b.path === '/dropped.ts')
    expect(openedBuffer).toBeDefined()
    const pane = windowPaneStore.getState().panes[ROOT_PANE_ID]
    expect(pane?.editorTabIds).toHaveLength(tabCountBefore + 1)
    expect(pane?.editorTabIds).toContain(openedBuffer!.id)
    expect(pane?.activeEditorTabId).toBe(openedBuffer!.id)
  })

  it("dropping a file on a pane's CENTER zone still opens it as a tab in that same pane (unchanged behavior)", async () => {
    const store = createWorkspaceStore('w1')

    useFileSystemStore.setState({
      handleFileOpen: async (path: string) => {
        windowPaneStore.getState().bufferActions.openContent({
          type: 'editor',
          path,
          name: path.split('/').pop() ?? path,
          content: '',
          workspaceId: 'w1',
        })
      },
    })

    await renderPane(store)

    const paneIdsBefore = Object.keys(windowPaneStore.getState().panes).sort()

    resolveDropTargetOverride.current = { paneId: ROOT_PANE_ID, zone: 'center' }
    window.__fileDragData = {
      type: 'file',
      path: '/center-dropped.ts',
      name: 'center-dropped.ts',
      isDir: false,
    }

    const container = document.querySelector('[data-pane-container]')!
    await act(async () => {
      fireEvent.mouseUp(container, { clientX: 400, clientY: 300 })
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(Object.keys(windowPaneStore.getState().panes).sort()).toEqual(paneIdsBefore)
    const openedBuffer = windowPaneStore
      .getState()
      .buffers.find((b) => b.path === '/center-dropped.ts')
    expect(openedBuffer).toBeDefined()
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.editorTabIds).toContain(
      openedBuffer!.id,
    )
  })

  it('a directory drop is ignored entirely — no tab, no pane change', async () => {
    const store = createWorkspaceStore('w1')
    const handleFileOpen = vi.fn(async () => {})
    useFileSystemStore.setState({ handleFileOpen })

    await renderPane(store)

    const paneIdsBefore = Object.keys(windowPaneStore.getState().panes).sort()
    resolveDropTargetOverride.current = { paneId: ROOT_PANE_ID, zone: 'left' }
    window.__fileDragData = { type: 'file', path: '/some-dir', name: 'some-dir', isDir: true }

    const container = document.querySelector('[data-pane-container]')!
    await act(async () => {
      fireEvent.mouseUp(container, { clientX: 5, clientY: 200 })
      await Promise.resolve()
    })

    expect(handleFileOpen).not.toHaveBeenCalled()
    expect(Object.keys(windowPaneStore.getState().panes).sort()).toEqual(paneIdsBefore)
  })
})

// Task 9 (sidebar restyle recovery batch 2): a follow-up to Task 1
// (c33a7a58), which fixed the tab styling INSIDE the identity row. The user's
// own live follow-up on that fix flagged the row's CONTAINER: `data-pane-container`
// painted no background at all, so the row showed the page body's translucent
// `--chrome-bg` vibrancy tint through it, while `data-pane-content` directly
// below painted an explicit opaque `bg-pane-background` fill AND was the only
// box with rounded top corners — a two-tone "gray header over white rounded
// content" look. The design canvas's ground truth is one shared background
// across the row and the content, with rounding/border/shadow enclosing both.
//
// These tests extend Task 6's own gutter/rounding coverage (see the
// `buildPaneContentStyle` describe blocks in pane-border.test.ts, whose
// fixtures — a full-edge single pane, an interior (no-edge) pane, and a
// collapsed sidebar — are reused here) up to the DOM: not just "does the pure
// function return the right style object" but "does the element that ACTUALLY
// carries that style object now enclose the tab bar too."
describe("PaneContainer — the identity row shares the pane's background/rounding (Task 9)", () => {
  afterEach(() => {
    sidebarOpenOverride.current = true
    setActiveWorkspaceStoreRef(null)
  })

  it('nests the tab-bar row inside the same painted box as the content — not an unstyled sibling of it', async () => {
    const store = createWorkspaceStore('w1')
    await renderPane(store)

    const sharedBox = document.querySelector('[data-pane-content]')!
    const row = screen.getByTestId('tab-bar-marker')
    expect(sharedBox.contains(row)).toBe(true)
    expect(sharedBox).toHaveClass('bg-pane-background')

    // The outer shell (drag/drop mechanics, the pane-hit ring, PANE_DROP_ATTR)
    // paints no background of its own — before this fix this is exactly where
    // the page body's translucent --chrome-bg tint bled through behind the row.
    const outer = document.querySelector('[data-pane-container]')!
    expect(outer.className).not.toMatch(/\bbg-/)
  })

  it("the shared box's rounding/border/gutter — not just data-pane-content alone — matches buildPaneContentStyle for the pane's actual edges (single pane, open left sidebar: right+bottom are real window edges)", async () => {
    const store = createWorkspaceStore('w1')
    await renderPane(store, ROOT_PANE_POSITION)

    const sharedBox = document.querySelector('[data-pane-content]')! as HTMLElement
    // sidebarPosition defaults to 'left' (default-settings.ts); no
    // SidebarProvider wraps this tree, so useSidebarOptional falls back to
    // `?? true` — matching pane-container.tsx's own fallback exactly.
    const expected = buildPaneContentStyle(ROOT_PANE_POSITION, 'left', false, true)
    const reference = document.createElement('div')
    Object.assign(reference.style, expected)
    expect(sharedBox.getAttribute('style')).toBe(reference.getAttribute('style'))

    // Named corners, so a regression here reads as "which corner broke," not
    // just "some style string changed": left/top are shielded/never-edge and
    // stay rounded+inset; right/bottom are real window edges and square off —
    // and the tab bar (inside sharedBox) is enclosed by all of it.
    expect(sharedBox.style.borderTopLeftRadius).toBe('var(--radius-lg)')
    // jsdom's CSSOM normalizes a bare '0' length to '0px' on read-back (the
    // object buildPaneContentStyle returns, asserted unitless in
    // pane-border.test.ts, is unaffected — this is purely how the DOM
    // serializes it once assigned).
    expect(sharedBox.style.borderTopRightRadius).toBe('0px')
    expect(sharedBox.style.marginLeft).toBe('4px')
    expect(sharedBox.style.marginRight).toBe('0px')
  })

  it('an interior pane (touches no window edge) rounds and insets all four corners — the common multi-pane case', async () => {
    const interior: PanePosition = { atLeft: false, atTop: false, atRight: false, atBottom: false }
    const store = createWorkspaceStore('w1')
    await renderPane(store, interior)

    const sharedBox = document.querySelector('[data-pane-content]')! as HTMLElement
    const expected = buildPaneContentStyle(interior, 'left', false, true)
    const reference = document.createElement('div')
    Object.assign(reference.style, expected)
    expect(sharedBox.getAttribute('style')).toBe(reference.getAttribute('style'))
    expect(sharedBox.style.borderTopLeftRadius).toBe('var(--radius-lg)')
    expect(sharedBox.style.borderBottomRightRadius).toBe('var(--radius-lg)')
    expect(sharedBox.style.marginLeft).toBe('4px')
    expect(sharedBox.style.marginBottom).toBe('4px')
  })

  // The regression this whole style object exists to prevent: a rounded,
  // shadowed corner composited against the window's own rounded vibrant edge
  // measured 8ms -> 106ms frames in WKWebView. Moving WHERE this style
  // attaches must not turn a real window edge into a rounded one — proven
  // here at the DOM level, not just against the pure function.
  it('still flattens the corner at a REAL window edge once the sidebar collapses — not just the common shielded case', async () => {
    sidebarOpenOverride.current = false
    const store = createWorkspaceStore('w1')
    await renderPane(store, ROOT_PANE_POSITION)

    const sharedBox = document.querySelector('[data-pane-content]')! as HTMLElement
    const expected = buildPaneContentStyle(ROOT_PANE_POSITION, 'left', false, false)
    // Sanity on the fixture itself: a collapsed sidebar turns the left edge
    // into a real window edge too.
    expect(expected.borderTopLeftRadius).toBe('0')
    expect(expected.marginLeft).toBe('0')

    const reference = document.createElement('div')
    Object.assign(reference.style, expected)
    expect(sharedBox.getAttribute('style')).toBe(reference.getAttribute('style'))
    expect(sharedBox.style.borderTopLeftRadius).toBe('0px') // see the unit note above
    // The longhand, not the `borderLeft` shorthand: jsdom's shorthand getter
    // doesn't reliably reconstruct a `border-style: none` sub-value (reports
    // the width instead) — borderLeftStyle is unambiguous, and the full-string
    // getAttribute('style') comparison above already proves the two elements'
    // serialized styles are byte-for-byte identical either way.
    expect(sharedBox.style.borderLeftStyle).toBe('none')
    expect(sharedBox.style.marginLeft).toBe('0px')

    // And the row is inside that exact, now-square box — not a separately
    // rounded sibling that would read as a still-detached header.
    const row = screen.getByTestId('tab-bar-marker')
    expect(sharedBox.contains(row)).toBe(true)
  })
})
