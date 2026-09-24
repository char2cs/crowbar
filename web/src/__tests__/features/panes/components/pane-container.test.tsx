import { createElement, Fragment, useEffect } from 'react'
import { useStore } from 'zustand'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { setActiveWorkspaceStoreForTests } from '@/features/workspace/stores/workspace-store-registry'
import {
  destroyWorkspaceStore,
  getAllActiveWorkspaceIds,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { ROOT_PANE_POSITION, type PanePosition } from '@/features/panes/types/pane'
import { buildPaneContentStyle } from '@/features/panes/utils/pane-border'
import { useSettingsStore } from '@/features/settings/store'
import { getDefaultSettingsSnapshot } from '@/features/settings/config/default-settings'

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
  TerminalPane: ({
    sessionId,
    bufferId,
    isActive,
    isVisible,
  }: {
    sessionId?: string
    bufferId: string
    isActive?: boolean
    isVisible?: boolean
  }) => {
    useEffect(() => {
      terminalMountCount.current += 1
    }, [])
    return createElement('div', {
      'data-testid': `terminal-marker-${bufferId}`,
      'data-session-id': sessionId ?? '',
      // xterm gates its render loop on these — a hidden view's terminal must
      // be told to stop, not merely covered up.
      'data-active': String(isActive ?? ''),
      'data-visible': String(isVisible ?? ''),
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

// Editor-portal fix: a mount counter, same reasoning as terminalMountCount
// below — proves the retained editor widget survives a switch to a
// non-editor tab (branch review) and back, not just that its marker div
// looks the same afterwards.
const { editorMountCount } = vi.hoisted(() => ({ editorMountCount: { current: 0 } }))
vi.mock('@/features/panes/components/editor-pane', () => ({
  EditorPane: ({
    bufferId,
    isPreview,
    isActiveSurface,
  }: {
    bufferId: string
    isPreview: boolean
    isActiveSurface: boolean
  }) => {
    useEffect(() => {
      editorMountCount.current += 1
    }, [])
    return createElement('div', {
      'data-testid': `editor-marker-${bufferId}`,
      'data-preview': String(isPreview),
      'data-active-surface': String(isActiveSurface),
    })
  },
}))

// Not exercised directly by this file's tests (BranchReviewPane has its own
// dedicated coverage) — only needed as a light stand-in so a pane can hold a
// non-editor tab to switch to, without pulling in its real diff-rendering.
vi.mock('@/features/git/components/branch-review-pane', () => ({
  BranchReviewPane: ({ wsId }: { wsId: string }) =>
    createElement('div', { 'data-testid': 'branch-review-marker', 'data-ws-id': wsId }),
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
import { nextVersion, seedChats } from '@/__tests__/__fixtures__/agent-chat'
import { EditorHostRegistry } from '@/features/panes/components/editor-host-registry'

function PaneHost({ position, showing }: { position?: PanePosition; showing?: boolean }) {
  // Task 26: panes are window-level now — read off windowPaneStore, not the
  // per-workspace WorkspaceStoreContext.
  const pane = useStore(windowPaneStore, (s) => s.panes[ROOT_PANE_ID])
  if (!pane) return null
  return createElement(PaneContainer, { pane, position, showing })
}

// `position` defaults to ROOT_PANE_POSITION (PaneContainer's own default) for
// every existing caller; Task 9's window-edge/interior-pane tests pass one
// explicitly to control which of the pane's own edges are real window edges.
//
// EditorHostRegistry is rendered as a SIBLING of PaneHost, matching
// production (WorkspaceLayoutRoot renders it alongside SplitViewRoot, not
// inside it — see editor-host-registry.tsx's own doc): PaneContainer no
// longer renders EditorPane directly, only a portal target div, so this is
// what actually connects the mocked EditorPane back into the tree the
// `editor-marker-*` assertions below query.
async function renderPane(
  store: ReturnType<typeof createWorkspaceStore>,
  position?: PanePosition,
  showing?: boolean,
) {
  await act(async () => {
    render(
      createElement(
        WorkspaceStoreContext.Provider,
        { value: store },
        createElement(Fragment, null, [
          createElement(PaneHost, { position, showing, key: 'pane' }),
          createElement(EditorHostRegistry, { key: 'editor-host' }),
        ]),
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

/** A branch-review tab — a non-editor buffer type, used by the editor-portal
 *  regression below to switch a pane's active tab AWAY from its editor
 *  buffer and back. */
function seedBranchReviewTab(
  _store: ReturnType<typeof createWorkspaceStore>,
  paneId: string,
  id: string,
) {
  windowPaneStore.setState((state) => {
    state.buffers.push({
      id,
      type: 'branchReview',
      name: 'Branch Review',
      wsId: 'w1',
      isPinned: false,
      workspaceId: 'w1',
    })
    return state
  })
  windowPaneStore.getState().paneActions.addEditorTabToPane(paneId, {
    id,
    type: 'branchReview',
    name: 'Branch Review',
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
    setActiveWorkspaceStoreForTests(null)
  })

  it('renders the chat, not NewTabView, when the pane has a chat and zero editor tabs', async () => {
    const store = createWorkspaceStore('w1')
    windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })

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

  // Chats/pane redesign: with zero editor tabs, there is no IDE sector at
  // all — TabBar is replaced outright by ChatOnlyPaneHeader, which takes
  // over its window-chrome (drag region) and right-pinned actions.
  it('replaces TabBar with ChatOnlyPaneHeader when the pane has a chat and zero editor tabs', async () => {
    const store = createWorkspaceStore('w1')
    windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })

    await renderPane(store)

    await screen.findByTestId('chat-chat-1')
    expect(screen.queryByTestId('tab-bar-marker')).not.toBeInTheDocument()
    const row = screen.getByTestId('pane-top-row')
    expect(row).toHaveAttribute('data-tauri-drag-region')
    expect(screen.getByTestId('chat-branch-header')).toBeInTheDocument()
  })

  // A user CAN toggle the split on with zero editor tabs (nothing gates
  // `editorOpen` on tab count) — this must not resurrect a side-by-side
  // split against an empty NewTabView. `chatFillsPane` overrides
  // `presentation` for exactly this case.
  it('still hides the editor view when editorOpen is on but there are zero editor tabs', async () => {
    const store = createWorkspaceStore('w1')
    windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })
    windowPaneStore.setState((state) => {
      const pane = state.panes[ROOT_PANE_ID]
      if (pane) pane.editorOpen = true
      return state
    })

    await renderPane(store)

    const chat = await screen.findByTestId('chat-chat-1')
    expect(screen.getByTestId('new-tab-marker').closest('[hidden]')).not.toBeNull()
    expect(chat.closest('[hidden]')).toBeNull()
    expect(screen.queryByTestId('tab-bar-marker')).not.toBeInTheDocument()
  })

  // The exact regression class this file's own render-tree comments obsess
  // over (see pane-container.tsx's "ONE STABLE PARENT" note): swapping
  // TabBar for ChatOnlyPaneHeader as tabs come and go must never touch the
  // chat view's own position/identity — only the chrome around it.
  it('never remounts the chat when its pane crosses zero editor tabs in either direction', async () => {
    const store = createWorkspaceStore('w1')
    windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })

    await renderPane(store)
    const chatWithZeroTabs = await screen.findByTestId('chat-chat-1')

    await act(async () => {
      seedEditorTab(store, ROOT_PANE_ID, 'tab-a')
    })
    const chatWithOneTab = screen.getByTestId('chat-chat-1')
    expect(chatWithOneTab).toBe(chatWithZeroTabs)
    // TabBar is back now that there's a tab to show.
    expect(screen.getByTestId('tab-bar-marker')).toBeInTheDocument()

    await act(async () => {
      windowPaneStore.getState().paneActions.removeEditorTabFromPane(ROOT_PANE_ID, 'tab-a')
    })
    const chatBackToZeroTabs = screen.getByTestId('chat-chat-1')
    expect(chatBackToZeroTabs).toBe(chatWithZeroTabs)
    expect(screen.queryByTestId('tab-bar-marker')).not.toBeInTheDocument()
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
    windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })
    // addEditorTabToPane sets pane.editorOpen = true as a side effect — the
    // split-toggle state that gates whether the editor view shows alongside
    // an existing chat (spec §7.1/§7.2).
    seedEditorTab(store, ROOT_PANE_ID, 'tab-a')

    await renderPane(store)

    expect(await screen.findByTestId('chat-chat-1')).toBeInTheDocument()
    expect(await screen.findByTestId('editor-marker-tab-a')).toBeInTheDocument()
  })

  it('a pane with both a chat and editor tabs, split toggled off, keeps both mounted — the just-activated tab shows, the chat hides', async () => {
    const store = createWorkspaceStore('w1')
    windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })
    // addEditorTabToPane activates the new tab as a side effect — in the
    // collapsed presentation that means the TAB is what's selected (chats/
    // pane redesign: the chat is "just another tab" here), not the chat.
    seedEditorTab(store, ROOT_PANE_ID, 'tab-a')
    // addEditorTabToPane also sets editorOpen = true — force the split back
    // off so this test exercises the collapsed ('tabs') presentation.
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
    // ...and, since a real tab is the one selected, IT shows — the chat's
    // content-area ancestor is the one carrying the native `hidden`
    // attribute (display:none via the UA stylesheet — not a Tailwind class,
    // so this assertion needs no compiled CSS to be meaningful) instead.
    expect(editorMarker.closest('[hidden]')).toBeNull()
    expect(chat.closest('[hidden]')).not.toBeNull()
  })

  it('opening another file/terminal into a pane whose split was toggled off does not reopen the split', async () => {
    const store = createWorkspaceStore('w1')
    windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })
    seedEditorTab(store, ROOT_PANE_ID, 'tab-a')
    windowPaneStore.setState((state) => {
      const pane = state.panes[ROOT_PANE_ID]
      if (pane) pane.editorOpen = false
      return state
    })

    // Opening a SECOND file into the same pane, with the split still
    // collapsed, must not force editorOpen back to true — regression for the
    // bug where addEditorTabToPane unconditionally reopened the split on
    // every file/terminal open, even when the pane already held a tab and
    // the user had deliberately collapsed it.
    seedEditorTab(store, ROOT_PANE_ID, 'tab-b')

    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.editorOpen).toBe(false)

    await renderPane(store)

    // Still the collapsed ('tabs') presentation: the newly-opened tab shows,
    // the chat hides — same as the single-tab case, just proving a second
    // open didn't flip editorOpen back to true and switch to a real split.
    const chat = await screen.findByTestId('chat-chat-1')
    const editorMarker = await screen.findByTestId('editor-marker-tab-b')
    expect(editorMarker.closest('[hidden]')).toBeNull()
    expect(chat.closest('[hidden]')).not.toBeNull()
  })

  it('selecting the chat in the collapsed presentation hides the editor view instead, still mounted', async () => {
    const store = createWorkspaceStore('w1')
    windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })
    seedEditorTab(store, ROOT_PANE_ID, 'tab-a')
    windowPaneStore.setState((state) => {
      const pane = state.panes[ROOT_PANE_ID]
      if (pane) pane.editorOpen = false
      return state
    })
    windowPaneStore.getState().paneActions.activateChatInPane(ROOT_PANE_ID)

    await renderPane(store)

    const chat = await screen.findByTestId('chat-chat-1')
    const editorMarker = await screen.findByTestId('editor-marker-tab-a')
    expect(editorMarker).toBeInTheDocument()
    expect(editorMarker.closest('[hidden]')).not.toBeNull()
    expect(chat.closest('[hidden]')).toBeNull()
  })

  it('keeps the chat mounted (same DOM node) across an editor-tab activation', async () => {
    const store = createWorkspaceStore('w1')
    windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })
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
      (windowPaneStore.getState().panes[ROOT_PANE_ID] as unknown as Record<string, unknown>)
        .previewBufferId,
    ).toBe(undefined)
  })

  it('a split-zone drop of a tab from a DIFFERENT pane is rejected — a tab may never cross a pane boundary via drag', async () => {
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

    // The dropped payload names 'phantom-source-pane' as its origin — a pane
    // other than the one rendered here — so this drop must be rejected
    // outright: never moved, never activated into this pane.
    expect(moveCalls).toHaveLength(0)
    expect(activateCalls).toHaveLength(0)
    // The old buffer-vocabulary actions this migration retired do not exist
    // on the actions object at all — a regression reintroducing them (or
    // pane-container calling them) would fail loudly, not silently no-op.
    const actions = windowPaneStore.getState().paneActions as unknown as Record<string, unknown>
    expect(actions.moveBufferToPane).toBeUndefined()
    expect(actions.activatePaneBuffer).toBeUndefined()
    expect(actions.addBufferToPane).toBeUndefined()
  })

  it('a center-zone drop of a tab from a DIFFERENT pane is rejected — the tab stays exactly where it was', async () => {
    const store = createWorkspaceStore('w1')
    // The center zone routes through pane-drop-actions.ts's
    // ensureBufferInPaneDropTarget, which reads the GLOBAL
    // active-workspace-store ref (a separate registry from the
    // WorkspaceStoreContext.Provider renderPane uses below) — the same setup
    // pane-drop-actions.test.ts already needs for that helper.
    setActiveWorkspaceStoreForTests(store)

    const sourcePaneId = windowPaneStore
      .getState()
      .paneActions.splitPane(ROOT_PANE_ID, 'horizontal')
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

    // A cross-pane tab drop is not a supported gesture: the tab must never
    // land in ROOT_PANE_ID, and must still be exactly where it started.
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.editorTabIds ?? []).not.toContain(
      'moved-tab',
    )
    expect(windowPaneStore.getState().panes[sourcePaneId]?.editorTabIds).toContain('moved-tab')
  })
})

// Bug: clicking into an inactive pane's chat/editor content sometimes never
// changes focus. handlePaneMouseDownCapture (the onMouseDownCapture wired to
// the pane's own root div) used `target.closest("button, input, textarea,
// [role='button'], [role='menu']")` to skip setActivePane — but .closest()
// walks the WHOLE ancestor chain, not just the literal mousedown target. Chat
// message content, Monaco's toolbar/find-bar and Plate's toolbar all nest
// real buttons throughout their content, so a mousedown anywhere inside one
// of those ancestors — even far from the literal button — was silently
// swallowed and the pane never activated. Separately, the dead
// `isEditorTextarea` escape hatch checked for a class ('editor-textarea')
// that is never applied anywhere — Monaco's own real input surface uses
// 'inputarea' (textAreaEditContext.js) — so a direct click on Monaco's real
// textarea was ALSO swallowed, unlike xterm's real helper textarea, whose
// override ('xterm-helper-textarea') is wired correctly.
describe('PaneContainer — mousedown-capture pane activation', () => {
  afterEach(() => {
    setActiveWorkspaceStoreForTests(null)
  })

  /** A second pane, so ROOT_PANE_ID can start inactive — same pattern the
   *  active-pane accent ring suite above already uses. */
  function makeRootPaneInactive(): void {
    const other = windowPaneStore
      .getState()
      .paneActions.splitPane(ROOT_PANE_ID, 'horizontal', undefined, 'after')!
    windowPaneStore.getState().paneActions.setActivePane(other)
  }

  it('activates an inactive pane on a mousedown whose target is merely NESTED inside a button, not the button itself', async () => {
    const store = createWorkspaceStore('w1')
    makeRootPaneInactive()

    await renderPane(store)
    expect(windowPaneStore.getState().activePaneId).not.toBe(ROOT_PANE_ID)

    // Stand-in for a real button deep in pane content (a chat message's copy
    // button, a Monaco/Plate toolbar icon, ...) — the literal mousedown
    // TARGET is a child of the button, never the button element itself.
    const paneContainer = document.querySelector(`[data-pane-id="${ROOT_PANE_ID}"]`) as HTMLElement
    const nestedButton = document.createElement('button')
    const icon = document.createElement('span')
    nestedButton.appendChild(icon)
    paneContainer.appendChild(nestedButton)

    fireEvent.mouseDown(icon)

    expect(windowPaneStore.getState().activePaneId).toBe(ROOT_PANE_ID)
  })

  it('activates the pane on a mousedown landing on Monaco’s own real inputarea, despite it being a literal <textarea>', async () => {
    const store = createWorkspaceStore('w1')
    makeRootPaneInactive()

    await renderPane(store)
    expect(windowPaneStore.getState().activePaneId).not.toBe(ROOT_PANE_ID)

    const paneContainer = document.querySelector(`[data-pane-id="${ROOT_PANE_ID}"]`) as HTMLElement
    const monacoTextarea = document.createElement('textarea')
    monacoTextarea.className = 'inputarea monaco-mouse-cursor-text'
    paneContainer.appendChild(monacoTextarea)

    fireEvent.mouseDown(monacoTextarea)

    expect(windowPaneStore.getState().activePaneId).toBe(ROOT_PANE_ID)
  })

  it('still skips activation when the mousedown target IS ITSELF a real interactive control — the preserved exception', async () => {
    const store = createWorkspaceStore('w1')
    makeRootPaneInactive()

    await renderPane(store)
    expect(windowPaneStore.getState().activePaneId).not.toBe(ROOT_PANE_ID)

    const paneContainer = document.querySelector(`[data-pane-id="${ROOT_PANE_ID}"]`) as HTMLElement
    const button = document.createElement('button')
    paneContainer.appendChild(button)

    fireEvent.mouseDown(button)

    expect(windowPaneStore.getState().activePaneId).not.toBe(ROOT_PANE_ID)
  })

  // Live bug report: "clicking on a full split tab (like monaco) in a split
  // chat view that has another chat focused doesn't focus the chat that has
  // that tab opened." The two tests above cover raw DOM nodes appended
  // directly under `[data-pane-id]` — a reasonable proxy for a literal
  // <textarea>, but NOT for the real EditorPane/Monaco widget, which is
  // never a React DESCENDANT of PaneContainer at all: editor-host-registry.tsx
  // portals it in from EditorHostRegistry, a REACT SIBLING of the whole pane
  // tree (rendered alongside SplitViewRoot in workspace-layout-root.tsx). Its
  // DOM node still lands inside this pane's own subtree (the portal TARGET
  // div PaneContainer publishes via editor-portal-registry.ts), but React's
  // synthetic event dispatch collects ancestor handlers by walking the FIBER
  // tree, not the DOM tree — a portal's bubble path goes to the portal's
  // React parent (EditorHostSlot), never to PaneContainer. That is why the
  // OLD `onMouseDownCapture`/`onClick` React props on this pane's own root
  // (still exercised above) never fired for a genuine click landing on
  // Monaco, no matter how many explicit `setActivePane` calls got sprinkled
  // into individual handlers elsewhere (tab-bar.tsx's tab-strip click, etc.)
  // — none of them sit on the click's REAL path. `renderPane` mounts
  // EditorHostRegistry as a true sibling here too, exactly like production,
  // so these tests exercise the real portal boundary, not a stand-in for it.
  it('activates an inactive pane on a mousedown landing inside the PORTALED editor surface — side by side with the chat (the reported "full split tab" case)', async () => {
    const store = createWorkspaceStore('w1')
    windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })
    seedEditorTab(store, ROOT_PANE_ID, 'tab-a')
    makeRootPaneInactive()

    await renderPane(store)
    expect(windowPaneStore.getState().activePaneId).not.toBe(ROOT_PANE_ID)

    const editorMarker = await screen.findByTestId('editor-marker-tab-a')
    fireEvent.mouseDown(editorMarker)

    expect(windowPaneStore.getState().activePaneId).toBe(ROOT_PANE_ID)
  })

  it('activates an inactive pane on a mousedown landing inside the portaled editor surface when the tab fills the WHOLE pane (collapsed presentation, editor selected over the chat)', async () => {
    const store = createWorkspaceStore('w1')
    windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })
    seedEditorTab(store, ROOT_PANE_ID, 'tab-a') // addEditorTabToPane also selects the tab over the chat
    windowPaneStore.setState((state) => {
      const pane = state.panes[ROOT_PANE_ID]
      if (pane) pane.editorOpen = false // collapses to the 'tabs' presentation
      return state
    })
    makeRootPaneInactive()

    await renderPane(store)
    expect(windowPaneStore.getState().activePaneId).not.toBe(ROOT_PANE_ID)

    const editorMarker = await screen.findByTestId('editor-marker-tab-a')
    // Sanity: this IS the surface actually on screen, not a hidden sibling.
    expect(editorMarker.closest('[hidden]')).toBeNull()
    fireEvent.mouseDown(editorMarker)

    expect(windowPaneStore.getState().activePaneId).toBe(ROOT_PANE_ID)
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
/**
 * The active-pane ring is drawn by a childless overlay that fades its OPACITY,
 * never by animating a colour on the shared pane box itself.
 *
 * That box is large, rounded and painted in translucent `--chrome-bg` over the
 * window's real vibrancy, so WebKit re-blends all of it on every frame a colour
 * on it interpolates: with `transition-colors` fading its border, one focus
 * click bought a ~150ms train of 17-26ms frames. Measured live in the Tauri
 * app, three panes tiled in one workspace view, 12 focus clicks: 85fps with the
 * border-colour fade against 116fps with it suppressed, for identical React
 * work — and neither `contain: paint` (82.9fps) nor forcing a compositing layer
 * (67.8fps) recovered any of it. The accent had to come off that surface; the
 * fade itself must stay, because an untransitioned swap is the hard colour snap
 * around the tab row that was reported live as "the tabs flash with something"
 * on every pane click.
 */
describe('PaneContainer — the active-pane accent ring', () => {
  const paneBox = () => document.querySelector('[data-pane-content]') as HTMLElement
  const accent = () => document.querySelector('[data-pane-accent]') as HTMLElement

  /** A second pane, so the ring is worth drawing at all — with one pane on
   *  screen there is nothing to distinguish it FROM (useVisiblePaneCount). */
  function splitSoTheRingApplies() {
    return windowPaneStore
      .getState()
      .paneActions.splitPane(ROOT_PANE_ID, 'horizontal', undefined, 'after')!
  }

  it('leaves the pane box its neutral border in BOTH states — only the overlay changes', async () => {
    const store = createWorkspaceStore('w1')
    windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })
    const other = splitSoTheRingApplies()
    windowPaneStore.getState().paneActions.setActivePane(ROOT_PANE_ID)

    await renderPane(store)

    expect(paneBox().style.borderTop).toBe('1px solid var(--border)')

    await act(async () => {
      windowPaneStore.getState().paneActions.setActivePane(other)
    })

    // The box NEVER swaps to --secondary — that swap is what repainted it.
    expect(paneBox().style.borderTop).toBe('1px solid var(--border)')
  })

  it('fades the overlay opacity between active and inactive, on the box’s own border geometry', async () => {
    const store = createWorkspaceStore('w1')
    windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })
    const other = splitSoTheRingApplies()
    windowPaneStore.getState().paneActions.setActivePane(ROOT_PANE_ID)

    await renderPane(store)

    expect(accent().style.borderTop).toBe('1px solid var(--secondary)')
    expect(accent().style.opacity).toBe('1')
    // Geometry is the SAME computation the box uses, so the ring lands exactly
    // on the neutral border it hides rather than beside it: the box's margins
    // are the overlay's insets, and every corner radius matches.
    expect(accent().style.left).toBe(paneBox().style.marginLeft)
    expect(accent().style.top).toBe(paneBox().style.marginTop)
    expect(accent().style.right).toBe(paneBox().style.marginRight)
    expect(accent().style.bottom).toBe(paneBox().style.marginBottom)
    expect(accent().style.borderTopLeftRadius).toBe(paneBox().style.borderTopLeftRadius)
    expect(accent().style.borderBottomRightRadius).toBe(paneBox().style.borderBottomRightRadius)
    // ...and it must be a transition, not a swap: an instant flip is the snap
    // this whole arrangement exists to keep as a fade.
    expect(accent().className).toContain('transition-opacity')

    await act(async () => {
      windowPaneStore.getState().paneActions.setActivePane(other)
    })

    expect(accent().style.opacity).toBe('0')
  })

  it('never rings a lone pane — there is nothing to distinguish it from', async () => {
    const store = createWorkspaceStore('w1')
    windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })
    windowPaneStore.getState().paneActions.setActivePane(ROOT_PANE_ID)

    await renderPane(store)

    expect(accent().style.opacity).toBe('0')
  })
})

describe('PaneContainer — chat/editor-view arrangement (spec §7.2)', () => {
  afterEach(() => {
    setActiveWorkspaceStoreForTests(null)
  })

  /** jsdom reports 0 for every element's clientWidth/clientHeight (no layout
   *  engine) — usePaneViewPresentation treats an unmeasured 0x0 pane as
   *  side-by-side (see its own "flash" comment), so that is the size this
   *  suite gets for free without overriding anything. */
  it('an unmeasured pane with the split on defaults to side by side: a divider, and the editor is not hidden', async () => {
    const store = createWorkspaceStore('w1')
    windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })
    seedEditorTab(store, ROOT_PANE_ID, 'tab-a') // addEditorTabToPane sets editorOpen = true

    await renderPane(store)

    const chat = await screen.findByTestId('chat-chat-1')
    const editorMarker = await screen.findByTestId('editor-marker-tab-a')
    expect(chat.closest('[hidden]')).toBeNull()
    expect(editorMarker.closest('[hidden]')).toBeNull()
    expect(screen.getByRole('separator')).toHaveAttribute('aria-orientation', 'vertical')
  })

  // The actual bug this locks in: TabBar is the IDE SECTOR's own header —
  // in side by side (and stacked), it must be confined to the editor's own
  // box, never spanning over the chat's column/row too.
  it('confines TabBar to the editor’s own box in side-by-side — never spanning over the chat column too', async () => {
    const store = createWorkspaceStore('w1')
    windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })
    seedEditorTab(store, ROOT_PANE_ID, 'tab-a')

    await renderPane(store)

    const chat = await screen.findByTestId('chat-chat-1')
    const tabBar = screen.getByTestId('tab-bar-marker')
    const editorView = document.querySelector('[data-editor-view]')! as HTMLElement
    const chatView = document.querySelector('[data-chat-view]')!

    expect(editorView.contains(tabBar)).toBe(true)
    expect(chatView.contains(tabBar)).toBe(false)
    expect(editorView.contains(chat)).toBe(false)

    // And the chat gets ITS OWN header, confined to its own column, in the
    // same box as the chat surface — not the editor's.
    expect(chatView.contains(screen.getByTestId('chat-branch-header'))).toBe(true)

    // The IDE sector reads as its own card next to the chat's: a border on
    // the edge that actually touches the chat (left, in side-by-side), never
    // rounded on any corner (buildInnerViewStyle).
    expect(editorView.style.borderLeft).toBe('1px solid var(--border)')
    expect(editorView.style.borderTopLeftRadius).toBe('0px') // jsdom normalizes '0' on read-back
    expect(editorView.style.borderBottomLeftRadius).toBe('0px')
    expect(editorView.style.borderTopRightRadius).toBe('0px')
    expect(editorView.style.borderBottomRightRadius).toBe('0px')
  })

  it('with the split off, there is no divider — tabs, not a cramped split', async () => {
    const store = createWorkspaceStore('w1')
    windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })
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
      windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })
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

  it('confines TabBar to the editor’s own box in stacked too, not spanning the chat above it', async () => {
    await withPaneBox(500, 1200, async () => {
      const store = createWorkspaceStore('w1')
      windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })
      seedEditorTab(store, ROOT_PANE_ID, 'tab-a')

      await renderPane(store)

      const chat = await screen.findByTestId('chat-chat-1')
      const tabBar = screen.getByTestId('tab-bar-marker')
      const editorView = document.querySelector('[data-editor-view]')! as HTMLElement
      const chatView = document.querySelector('[data-chat-view]')!

      expect(editorView.contains(tabBar)).toBe(true)
      expect(chatView.contains(tabBar)).toBe(false)
      expect(editorView.contains(chat)).toBe(false)
      expect(chatView.contains(screen.getByTestId('chat-branch-header'))).toBe(true)

      // Stacked: the chat sits ABOVE the editor, so the border/rounding
      // belongs on the TOP edge here, not the left.
      const outerRef = document.createElement('div')
      Object.assign(outerRef.style, buildPaneContentStyle(ROOT_PANE_POSITION, 'left', false, true))
      expect(editorView.style.borderTop).toBe('1px solid var(--border)')
      // TL: facing (top) + left (shielded/interior for ROOT_PANE_POSITION
      // with a left sidebar) -> squares. Stacked's own top seam never rounds
      // its own two corners (doubling bug fix, pane-border.ts) — its left
      // border already runs doubled against the outer box's identical one
      // for the whole view's height here, so rounding away would read as a
      // second, isolated corner rather than one line.
      expect(editorView.style.borderTopLeftRadius).toBe('0px') // jsdom normalizes '0' on read-back
      // TR: facing (top) + right (a REAL window edge for ROOT_PANE_POSITION
      // with a left sidebar) -> squares too, unchanged from before.
      expect(editorView.style.borderTopRightRadius).toBe('0px') // jsdom normalizes '0' on read-back
      expect(editorView.style.borderBottomLeftRadius).toBe(outerRef.style.borderBottomLeftRadius)
      expect(editorView.style.borderBottomRightRadius).toBe(outerRef.style.borderBottomRightRadius)
    })
  })

  // The chat sits next to wherever the sidebar is — a sidebar on the right
  // means the chat renders on the right too, so the two never end up on
  // opposite edges of the window. The rounded, bordered edge of the IDE
  // sector always faces the chat, whichever side that ends up being.
  describe('the chat follows the sidebar side (side by side)', () => {
    afterEach(() => {
      useSettingsStore.setState({ settings: getDefaultSettingsSnapshot() })
    })

    it('sidebar on the right: the chat renders AFTER the editor, and the editor rounds/borders its RIGHT edge', async () => {
      useSettingsStore.setState((s) => ({
        settings: { ...s.settings, sidebarPosition: 'right' },
      }))
      const store = createWorkspaceStore('w1')
      windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })
      seedEditorTab(store, ROOT_PANE_ID, 'tab-a')

      await renderPane(store)

      await screen.findByTestId('chat-chat-1')
      const chatView = document.querySelector('[data-chat-view]')!
      const editorView = document.querySelector('[data-editor-view]')! as HTMLElement

      // DOCUMENT_POSITION_FOLLOWING on chatView (from editorView's
      // perspective) means editorView comes first in the DOM.
      expect(
        Boolean(editorView.compareDocumentPosition(chatView) & Node.DOCUMENT_POSITION_FOLLOWING),
      ).toBe(true)
      expect(editorView.style.borderRight).toBe('1px solid var(--border)')
      expect(editorView.style.borderTopRightRadius).toBe('0px') // jsdom normalizes '0' on read-back
      expect(editorView.style.borderBottomRightRadius).toBe('0px')
      expect(editorView.style.borderTopLeftRadius).toBe('0px')
      expect(editorView.style.borderBottomLeftRadius).toBe('0px')
    })

    it('sidebar on the left (default): unchanged — the chat renders BEFORE the editor, rounding faces left', async () => {
      const store = createWorkspaceStore('w1')
      windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })
      seedEditorTab(store, ROOT_PANE_ID, 'tab-a')

      await renderPane(store)

      await screen.findByTestId('chat-chat-1')
      const chatView = document.querySelector('[data-chat-view]')!
      const editorView = document.querySelector('[data-editor-view]')! as HTMLElement

      expect(
        Boolean(chatView.compareDocumentPosition(editorView) & Node.DOCUMENT_POSITION_FOLLOWING),
      ).toBe(true)
      expect(editorView.style.borderLeft).toBe('1px solid var(--border)')
      expect(editorView.style.borderTopLeftRadius).toBe('0px') // jsdom normalizes '0' on read-back
      expect(editorView.style.borderBottomLeftRadius).toBe('0px')
    })

    it('stacked keeps the chat on TOP regardless of sidebar side — there is no left/right there', async () => {
      useSettingsStore.setState((s) => ({
        settings: { ...s.settings, sidebarPosition: 'right' },
      }))
      await withPaneBox(500, 1200, async () => {
        const store = createWorkspaceStore('w1')
        windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })
        seedEditorTab(store, ROOT_PANE_ID, 'tab-a')

        await renderPane(store)

        await screen.findByTestId('chat-chat-1')
        const chatView = document.querySelector('[data-chat-view]')!
        const editorView = document.querySelector('[data-editor-view]')! as HTMLElement

        expect(
          Boolean(chatView.compareDocumentPosition(editorView) & Node.DOCUMENT_POSITION_FOLLOWING),
        ).toBe(true)
        const outerRef = document.createElement('div')
        Object.assign(
          outerRef.style,
          buildPaneContentStyle(ROOT_PANE_POSITION, 'right', false, true),
        )
        expect(editorView.style.borderTop).toBe('1px solid var(--border)')
        // TL: facing (top) + left (a REAL window edge here — the sidebar
        // moved to the right, so left is no longer shielded) -> squares.
        expect(editorView.style.borderTopLeftRadius).toBe('0px') // jsdom normalizes '0' on read-back
        // TR: facing (top) + right (shielded/interior, sidebar now on the
        // right) -> squares too now (doubling bug fix, pane-border.ts):
        // stacked's own top seam never rounds its own two corners, since an
        // interior other-edge here means this box's right border already
        // runs doubled against the outer box's identical one for the whole
        // view's height.
        expect(editorView.style.borderTopRightRadius).toBe('0px') // jsdom normalizes '0' on read-back
        expect(editorView.style.borderBottomLeftRadius).toBe(outerRef.style.borderBottomLeftRadius)
        expect(editorView.style.borderBottomRightRadius).toBe(
          outerRef.style.borderBottomRightRadius,
        )
      })
    })
  })

  it('a landscape pane presents the two views side by side, with a vertical-orientation divider', async () => {
    await withPaneBox(1600, 500, async () => {
      const store = createWorkspaceStore('w1')
      windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })
      seedEditorTab(store, ROOT_PANE_ID, 'tab-a')

      await renderPane(store)

      await screen.findByTestId('chat-chat-1')
      expect(screen.getByRole('separator')).toHaveAttribute('aria-orientation', 'vertical')
    })
  })

  it('too small on both axes falls back to tabs even with the split on', async () => {
    await withPaneBox(300, 200, async () => {
      const store = createWorkspaceStore('w1')
      windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })
      // addEditorTabToPane activates the new tab — in the collapsed
      // ('tabs') presentation this falls back to, that means the TAB is
      // what's selected (chats/pane redesign), so the chat is the one that
      // hides, not the editor — see the same shift documented on the
      // "split toggled off" test above.
      seedEditorTab(store, ROOT_PANE_ID, 'tab-a')

      await renderPane(store)

      const chat = await screen.findByTestId('chat-chat-1')
      const editorMarker = await screen.findByTestId('editor-marker-tab-a')
      expect(editorMarker.closest('[hidden]')).toBeNull()
      expect(chat.closest('[hidden]')).not.toBeNull()
      expect(screen.queryByRole('separator')).not.toBeInTheDocument()
    })
  })

  it('never unmounts either view across an editorOpen toggle — same DOM nodes throughout', async () => {
    const store = createWorkspaceStore('w1')
    windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })
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
  // `pane.chatId` toggled. Reachable today: a chat filling a pane that already
  // holds editor tabs.
  it('does not remount the editor view — including a live terminal — when a chat lands in the pane', async () => {
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
      windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })
    })

    await screen.findByTestId('chat-chat-1')
    expect(screen.getByTestId('terminal-marker-term-a')).toBe(terminalBefore)
    expect(terminalMountCount.current).toBe(1) // still exactly one mount, ever
  })

  // Editor-portal fix: live-reported regression — switching a pane's active
  // tab from an editor buffer to a non-editor one (branch review) and back
  // used to fully unmount EditorPane (PaneContainer only ever rendered the
  // ACTIVE buffer's component), which disposed the pane's retained Monaco
  // widget and every model it held. Switching back created a brand-new
  // editor from scratch — occasionally landing blank or throwing, because
  // Monaco's own internal async work (e.g. its word-highlighter) could still
  // be in flight against the just-disposed instance. EditorHostRegistry now
  // owns EditorPane outside PaneContainer's own subtree entirely, so this
  // tab switch never reaches it — only the portal target's visibility
  // changes.
  it('does not remount the retained editor when the active tab switches to a non-editor buffer and back', async () => {
    editorMountCount.current = 0
    const store = createWorkspaceStore('w1')
    seedEditorTab(store, ROOT_PANE_ID, 'tab-a')
    seedBranchReviewTab(store, ROOT_PANE_ID, 'review-1')

    await renderPane(store)
    await act(async () => {
      windowPaneStore.getState().paneActions.activateEditorTabInPane(ROOT_PANE_ID, 'tab-a')
    })

    const editorBefore = await screen.findByTestId('editor-marker-tab-a')
    expect(editorMountCount.current).toBe(1)
    expect((editorBefore.parentElement as HTMLElement).style.visibility).not.toBe('hidden')

    // Switch away to the non-editor tab.
    await act(async () => {
      windowPaneStore.getState().paneActions.activateEditorTabInPane(ROOT_PANE_ID, 'review-1')
    })

    await screen.findByTestId('branch-review-marker')
    // Still the SAME node — never unmounted — just hidden.
    expect(screen.getByTestId('editor-marker-tab-a')).toBe(editorBefore)
    expect(editorMountCount.current).toBe(1)
    expect((editorBefore.parentElement as HTMLElement).style.visibility).toBe('hidden')

    // Switch back to the editor tab.
    await act(async () => {
      windowPaneStore.getState().paneActions.activateEditorTabInPane(ROOT_PANE_ID, 'tab-a')
    })

    expect(screen.getByTestId('editor-marker-tab-a')).toBe(editorBefore)
    expect(editorMountCount.current).toBe(1) // never remounted, ever
    expect((editorBefore.parentElement as HTMLElement).style.visibility).not.toBe('hidden')
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
    setActiveWorkspaceStoreForTests(null)
  })

  it('nests the tab-bar row inside the same rounded/clipped box as the content — not an unstyled sibling of it', async () => {
    const store = createWorkspaceStore('w1')
    await renderPane(store)

    const sharedBox = document.querySelector('[data-pane-content]')!
    const row = screen.getByTestId('tab-bar-marker')
    expect(sharedBox.contains(row)).toBe(true)
    // Chats/pane redesign feedback: the shared box paints the chat's own
    // translucent `bg-pane-chrome-bg` — giving each non-opaque region (the
    // chat view, the sash) its own copy of that fill left visible seams at
    // every boundary a caller forgot to cover explicitly. The IDE sector
    // still reads fully opaque: TabBar's real row paints `bg-pane-background`
    // OVER this fill within its own bounds (tab-bar.test.tsx covers that
    // directly; it's mocked away here). Distinct from `bg-chrome-bg` (body's
    // own wash, which the sidebar shows through) since the two surfaces now
    // carry different opacities.
    expect(sharedBox).toHaveClass('bg-pane-chrome-bg')

    // The outer shell (drag/drop mechanics, the pane-hit ring, PANE_DROP_ATTR)
    // paints no background of its own either — same reasoning as above, one
    // level further out.
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
    // stay rounded; right/bottom are real window edges and square off — and
    // the tab bar (inside sharedBox) is enclosed by all of it. Left sits
    // flush against the sidebar (no gutter) even though it stays rounded —
    // only top keeps its own inset.
    expect(sharedBox.style.borderTopLeftRadius).toBe('var(--radius-lg)')
    // jsdom's CSSOM normalizes a bare '0' length to '0px' on read-back (the
    // object buildPaneContentStyle returns, asserted unitless in
    // pane-border.test.ts, is unaffected — this is purely how the DOM
    // serializes it once assigned).
    expect(sharedBox.style.borderTopRightRadius).toBe('0px')
    expect(sharedBox.style.marginLeft).toBe('0px')
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
    expect(sharedBox.style.marginLeft).toBe('1px')
    expect(sharedBox.style.marginBottom).toBe('1px')
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

  // Reported live as "the tabs at the top flash with something for a
  // moment" on every pane click: buildPaneContentStyle swaps this box's
  // border between --border and --secondary as the active pane changes
  // (the ring answering "which of these has focus"), written straight into
  // `style`, and nothing gave that swap a transition — a same-frame color
  // snap right around the tab row it encloses. `sharedBox` is the exact box
  // the two `buildPaneContentStyle` DOM tests above prove carries that
  // border, so this only needs to prove IT also carries the class that
  // turns the snap into a fade.
  it("the shared box's active/inactive border swap is transitioned, not a same-frame snap", async () => {
    const store = createWorkspaceStore('w1')
    await renderPane(store)

    const sharedBox = document.querySelector('[data-pane-content]')!
    expect(sharedBox).toHaveClass('transition-colors')
  })
})

/**
 * A pane whose VIEW is parked stays mounted — that is the whole point, so
 * nothing it holds is torn down — but it must do no work. Hiding it without
 * telling the surfaces inside would be the cosmetic half of dormancy: the
 * xterm render loop keeps running against a `display: none` subtree, and the
 * chat pane's dormant-chat revive keeps thinking it is on screen and spawns a
 * vendor CLI for a view nobody is looking at.
 */
describe('PaneContainer — a pane in a view that is not on screen', () => {
  it('tells the chat surface it is not visible, and not the active pane', async () => {
    const store = createWorkspaceStore('w1')
    windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })

    await renderPane(store, undefined, false)

    const chat = screen.getByTestId('chat-chat-1')
    expect(chat.getAttribute('data-visible')).toBe('false')
    // `activePaneId` names one pane for the whole window; a view that is off
    // screen has no claim on it however that field happens to read.
    expect(chat.getAttribute('data-active-pane')).toBe('false')
  })

  it('tells a terminal to stop — it is never visible from behind another view', async () => {
    const store = createWorkspaceStore('w1')
    seedTerminalTab(store, ROOT_PANE_ID, 'term-1')

    await renderPane(store, undefined, false)

    const term = screen.getByTestId('terminal-marker-term-1')
    expect(term.getAttribute('data-visible')).toBe('false')
    expect(term.getAttribute('data-active')).toBe('false')
  })

  it('still mounts everything — parked is not closed', async () => {
    const store = createWorkspaceStore('w1')
    windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })
    seedTerminalTab(store, ROOT_PANE_ID, 'term-1')

    await renderPane(store, undefined, false)

    // Unmounting instead would dispose the xterm (blank on remount, scrollback
    // gone) and refcount-release the Monaco models with their undo history.
    expect(screen.getByTestId('chat-chat-1')).toBeInTheDocument()
    expect(screen.getByTestId('terminal-marker-term-1')).toBeInTheDocument()
  })

  it('the showing pane is told the opposite', async () => {
    const store = createWorkspaceStore('w1')
    windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })

    await renderPane(store, undefined, true)

    expect(screen.getByTestId('chat-chat-1').getAttribute('data-visible')).toBe('true')
  })
})

/**
 * WHOSE WORKSPACE a pane's chat belongs to.
 *
 * Panes are window-level (Task 26), so a drop can put ANY workspace's chat in
 * one — but a chat's own state and every chat-scoped URL are still
 * workspace-keyed. This used to read the AMBIENT `WorkspaceStoreContext` (the
 * `WorkspaceView` that happens to be rendering the pane), so a chat from
 * another workspace was resolved against the wrong store: never found, never
 * attached, permanently blank — and, since the pane persists, blank
 * across reload. That is the gap `openChatIntoPane`'s active-workspace
 * refusal stood in for, and closing it is what makes a cross-workspace drag
 * land.
 */
describe('PaneContainer — the chat’s own workspace, not the ambient one', () => {
  afterEach(() => {
    getAllActiveWorkspaceIds().forEach((id) => destroyWorkspaceStore(id))
  })

  const chatRecord = (id: string, wsId: string) => ({
    id,
    workspaceId: wsId,
    title: id,
    liveRunnerId: '',
    terminalSessionId: '',
    activeProviderId: 'claude',
    working: false,
    version: nextVersion(),
    phase: 'dormant' as const,
    createdAt: '2026-01-01T00:00:00Z',
    order: 0,
    parentId: '',
  })

  it('hands the chat surface the workspace the CHAT belongs to', async () => {
    seedChats(getOrCreateWorkspaceStore('w-owner'), [chatRecord('chat-1', 'w-owner')])
    windowPaneStore
      .getState()
      .paneActions.openChat('chat-1', { runnerId: 'runner-1', workspaceId: 'w-owner' })

    // Rendered under a DIFFERENT workspace's context — the one on screen.
    await renderPane(createWorkspaceStore('w-onscreen'))

    expect(await screen.findByTestId('chat-chat-1')).toHaveAttribute('data-ws-id', 'w-owner')
  })

  it('falls back to the ambient workspace while nothing can name an owner yet', async () => {
    windowPaneStore.getState().paneActions.openChat('chat-unknown')

    await renderPane(createWorkspaceStore('w-onscreen'))

    expect(await screen.findByTestId('chat-chat-unknown')).toHaveAttribute(
      'data-ws-id',
      'w-onscreen',
    )
  })

  // ChatBranchHeader (the chat's own identity header, mounted alongside
  // AgentChatPane) reads off the exact same resolved `chatStore` — this is
  // the same cross-workspace-title bug class, now checked at its new home.
  it("shows the chat's own title in its header even while a DIFFERENT workspace is ambient", async () => {
    seedChats(getOrCreateWorkspaceStore('w-owner'), [chatRecord('chat-1', 'w-owner')])
    windowPaneStore
      .getState()
      .paneActions.openChat('chat-1', { runnerId: 'runner-1', workspaceId: 'w-owner' })

    await renderPane(createWorkspaceStore('w-onscreen'))

    expect(await screen.findByTestId('chat-branch-header')).toHaveTextContent('chat-1')
  })

  // Live-reported: clicking "Review this branch" on an INACTIVE pane opened a
  // DIFFERENT chat's branch review — root cause was this exact ambient-vs-
  // owner confusion, but for a DATA-CREATING action (a persisted branchReview
  // buffer) rather than a display value. Unlike the header/title cases above,
  // a wrong answer here doesn't just render wrong and self-correct next
  // frame: it permanently tags a buffer with the wrong workspace.
  it("opens branch review for the CHAT's own workspace, not whichever one is ambient", async () => {
    seedChats(getOrCreateWorkspaceStore('w-owner'), [chatRecord('chat-1', 'w-owner')])
    windowPaneStore
      .getState()
      .paneActions.openChat('chat-1', { runnerId: 'runner-1', workspaceId: 'w-owner' })

    // Rendered under a DIFFERENT workspace's context — e.g. a split's other
    // pane, or whichever WorkspaceView happens to be on screen.
    await renderPane(createWorkspaceStore('w-onscreen'))

    fireEvent.click(await screen.findByTestId('branch-review-shortcut'))

    const buffers = windowPaneStore.getState().buffers
    const review = buffers.find((b) => b.type === 'branchReview')
    expect(review).toBeDefined()
    expect((review as { wsId?: string }).wsId).toBe('w-owner')
  })

  // The "skip rather than guess" half of the same fix (pane-chat-workspace.ts's
  // own documented principle): while nothing can yet name the chat's real
  // workspace, the action must no-op — never fall back to the ambient one and
  // silently create a buffer tagged with a workspace the chat doesn't belong
  // to at all.
  it("does not open branch review for the ambient workspace while the chat's own owner is still unresolved", async () => {
    windowPaneStore.getState().paneActions.openChat('chat-unknown')

    await renderPane(createWorkspaceStore('w-onscreen'))

    fireEvent.click(await screen.findByTestId('branch-review-shortcut'))

    expect(
      windowPaneStore.getState().buffers.find((b) => b.type === 'branchReview'),
    ).toBeUndefined()
  })
})
