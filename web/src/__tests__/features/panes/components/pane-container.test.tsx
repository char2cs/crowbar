import { createElement } from 'react'
import { act, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import {
  WorkspaceStoreContext,
  useWorkspaceStoreContext,
} from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'

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
// a plain button, so a drag-drop test can fire it without simulating HTML5 DnD
// through the real overlay's own zone-geometry math.
vi.mock('@/features/panes/components/split-drop-overlay', () => ({
  SplitDropOverlay: ({ onDrop }: { onDrop: (zone: string, e: unknown) => void }) =>
    createElement('button', {
      type: 'button',
      'data-testid': 'split-drop-trigger',
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

describe('PaneContainer — chat/editor-view hosting', () => {
  it('renders the chat, not NewTabView, when the pane has a chat and zero editor tabs', async () => {
    const store = createWorkspaceStore('w1')
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')

    await renderPane(store)

    const chat = await screen.findByTestId('chat-chat-1')
    expect(chat).toHaveAttribute('data-runner-id', 'runner-1')
    expect(chat).toHaveAttribute('data-ws-id', 'w1')
    expect(chat).toHaveAttribute('data-visible', 'true')
    expect(screen.queryByTestId('new-tab-marker')).not.toBeInTheDocument()
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

  it("reads the active tab's preview styling from its own isPreview field", async () => {
    const store = createWorkspaceStore('w1')
    seedEditorTab(store, ROOT_PANE_ID, 'tab-preview', { isPreview: true })

    await renderPane(store)

    expect(await screen.findByTestId('editor-marker-tab-preview')).toHaveAttribute(
      'data-preview',
      'true',
    )
  })

  it("does not read preview styling off a removed pane-level id", async () => {
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

    const trigger = await screen.findByTestId('split-drop-trigger')
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
})
