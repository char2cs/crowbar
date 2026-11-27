import { createElement, Fragment, useEffect } from 'react'
import { useStore } from 'zustand'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import {
  destroyWorkspaceStore,
  getAllActiveWorkspaceIds,
  getOrCreateWorkspaceStore,
  setActiveWorkspaceId,
} from '@/features/workspace/stores/workspace-store-registry'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { resetWindowPaneStoreForTests } from '@/__tests__/__fixtures__/window-pane-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { ROOT_PANE_POSITION, type PanePosition } from '@/features/panes/types/pane'
import { buildPaneContentStyle } from '@/features/panes/utils/pane-border'
import { useSettingsStore } from '@/features/settings/store'
import { getDefaultSettingsSnapshot } from '@/features/settings/config/default-settings'

import * as sidebar from '@/components/ui/sidebar'
import * as terminalPane from '@/features/panes/components/terminal-pane'
import * as chatPaneGate from '@/features/agent/components/agent-chat-pane-gate'
import * as editorPane from '@/features/panes/components/editor-pane'
import * as newTabView from '@/features/panes/components/new-tab-view'
import * as tabBar from '@/features/tabs/components/tab-bar'
import * as splitDropOverlay from '@/features/panes/components/split-drop-overlay'

const sidebarOpenOverride = { current: true }
vi.spyOn(sidebar, 'useSidebarOptional').mockImplementation(
  () => ({ open: sidebarOpenOverride.current }) as ReturnType<typeof sidebar.useSidebarOptional>,
)

const terminalMountCount = { current: 0 }
vi.spyOn(terminalPane, 'TerminalPane').mockImplementation(
  ({ sessionId, bufferId, isActive, isVisible }) => {
    useEffect(() => {
      terminalMountCount.current += 1
    }, [])
    return createElement('div', {
      'data-testid': `terminal-marker-${bufferId}`,
      'data-session-id': sessionId ?? '',
      'data-active': String(isActive ?? ''),
      'data-visible': String(isVisible ?? ''),
    })
  },
)

vi.spyOn(chatPaneGate, 'AgentChatPaneGate').mockImplementation(
  ({ chatId, runnerId, wsId, isActivePane, isVisible }) =>
    createElement('div', {
      'data-testid': `chat-${chatId}`,
      'data-runner-id': runnerId,
      'data-ws-id': wsId,
      'data-active-pane': String(isActivePane),
      'data-visible': String(isVisible),
    }),
)

const editorMountCount = { current: 0 }
vi.spyOn(editorPane, 'EditorPane').mockImplementation(({ bufferId, isActiveSurface }) => {
  useEffect(() => {
    editorMountCount.current += 1
  }, [])
  return createElement('div', {
    'data-testid': `editor-marker-${bufferId}`,
    'data-active-surface': String(isActiveSurface),
  })
})

vi.mock('@/features/git/components/branch-review-pane', () => ({
  BranchReviewPane: ({ wsId }: { wsId: string }) =>
    createElement('div', { 'data-testid': 'branch-review-marker', 'data-ws-id': wsId }),
}))

vi.spyOn(newTabView, 'NewTabView').mockImplementation(({ paneId, wsId }) =>
  createElement('div', {
    'data-testid': 'new-tab-marker',
    'data-pane-id': paneId ?? '',
    'data-ws-id': wsId ?? '',
  }),
)

vi.spyOn(tabBar, 'default').mockImplementation(() =>
  createElement('div', { 'data-testid': 'tab-bar-marker' }),
)

const centerDropPayload = {
  current: { bufferId: 'existing-tab', paneId: 'phantom-source-pane' },
}
vi.spyOn(splitDropOverlay, 'SplitDropOverlay').mockImplementation(({ onDrop }) =>
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
        } as never),
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
        } as never),
    }),
  ),
)

import { PaneContainer } from '@/features/panes/components/pane-container'
import { nextVersion, seedChats } from '@/__tests__/__fixtures__/agent-chat'
import { EditorHostRegistry } from '@/features/panes/components/editor-host-registry'

function PaneHost({ position, showing }: { position?: PanePosition; showing?: boolean }) {
  const pane = useStore(windowPaneStore, (s) => s.panes[ROOT_PANE_ID])
  if (!pane) return null
  return createElement(PaneContainer, { pane, position, showing })
}

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
      authority: 'workspace',
      path: `/${id}.ts`,
      name: `${id}.ts`,
      isDirty: false,
      isVirtual: false,
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

beforeEach(() => {
  resetWindowPaneStoreForTests()
})

describe('PaneContainer — chat/editor-view hosting', () => {
  afterEach(() => {
    setActiveWorkspaceId(null)
  })

  it('renders the chat, not NewTabView, when the pane has a chat and zero editor tabs', async () => {
    const store = createWorkspaceStore('w1')
    windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })

    await renderPane(store)

    const chat = await screen.findByTestId('chat-chat-1')
    expect(chat).toHaveAttribute('data-runner-id', 'runner-1')
    expect(chat).toHaveAttribute('data-ws-id', 'w1')
    expect(chat).toHaveAttribute('data-visible', 'true')
    expect(screen.getByTestId('new-tab-marker')).not.toBeVisible()
    expect(chat).toBeVisible()
  })

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

  it('shows the empty editor view when editorOpen is on with zero editor tabs', async () => {
    const store = createWorkspaceStore('w1')
    windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })
    windowPaneStore.setState((state) => {
      const pane = state.panes[ROOT_PANE_ID]
      if (pane) pane.editorOpen = true
      return state
    })

    await renderPane(store)

    const chat = await screen.findByTestId('chat-chat-1')
    expect(screen.getByTestId('new-tab-marker').closest('[hidden]')).toBeNull()
    expect(chat.closest('[hidden]')).toBeNull()
  })

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
    seedEditorTab(store, ROOT_PANE_ID, 'tab-a')

    await renderPane(store)

    expect(await screen.findByTestId('chat-chat-1')).toBeInTheDocument()
    expect(await screen.findByTestId('editor-marker-tab-a')).toBeInTheDocument()
  })

  it('a pane with both a chat and editor tabs, split toggled off, keeps both mounted — the just-activated tab shows, the chat hides', async () => {
    const store = createWorkspaceStore('w1')
    windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })
    seedEditorTab(store, ROOT_PANE_ID, 'tab-a')
    windowPaneStore.setState((state) => {
      const pane = state.panes[ROOT_PANE_ID]
      if (pane) pane.editorOpen = false
      return state
    })

    await renderPane(store)

    const chat = await screen.findByTestId('chat-chat-1')
    const editorMarker = await screen.findByTestId('editor-marker-tab-a')
    expect(editorMarker).toBeInTheDocument()
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

    seedEditorTab(store, ROOT_PANE_ID, 'tab-b')

    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.editorOpen).toBe(false)

    await renderPane(store)

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
    expect(chatAfter).toBe(chatBefore)
  })

  it('a split-zone drop of a tab from a DIFFERENT pane is rejected — a tab may never cross a pane boundary via drag', async () => {
    const store = createWorkspaceStore('w1')
    await renderPane(store)

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

    expect(moveCalls).toHaveLength(0)
    expect(activateCalls).toHaveLength(0)
    const actions = windowPaneStore.getState().paneActions as unknown as Record<string, unknown>
    expect(actions.moveBufferToPane).toBeUndefined()
    expect(actions.activatePaneBuffer).toBeUndefined()
    expect(actions.addBufferToPane).toBeUndefined()
  })

  it('a center-zone drop of a tab from a DIFFERENT pane is rejected — the tab stays exactly where it was', async () => {
    const store = getOrCreateWorkspaceStore('w1')
    setActiveWorkspaceId('w1')

    const sourcePaneId = windowPaneStore
      .getState()
      .paneActions.splitPane(ROOT_PANE_ID, 'horizontal')
    if (!sourcePaneId) throw new Error('splitPane did not create a source pane')
    windowPaneStore.setState((state) => {
      state.buffers.push({
        id: 'moved-tab',
        type: 'editor',
        authority: 'workspace',
        path: '/moved.ts',
        name: 'moved.ts',
        isDirty: false,
        isVirtual: false,
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

    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.editorTabIds ?? []).not.toContain(
      'moved-tab',
    )
    expect(windowPaneStore.getState().panes[sourcePaneId]?.editorTabIds).toContain('moved-tab')
  })
})

describe('PaneContainer — mousedown-capture pane activation', () => {
  afterEach(() => {
    setActiveWorkspaceId(null)
  })

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
    seedEditorTab(store, ROOT_PANE_ID, 'tab-a')
    windowPaneStore.setState((state) => {
      const pane = state.panes[ROOT_PANE_ID]
      if (pane) pane.editorOpen = false
      return state
    })
    makeRootPaneInactive()

    await renderPane(store)
    expect(windowPaneStore.getState().activePaneId).not.toBe(ROOT_PANE_ID)

    const editorMarker = await screen.findByTestId('editor-marker-tab-a')
    expect(editorMarker.closest('[hidden]')).toBeNull()
    fireEvent.mouseDown(editorMarker)

    expect(windowPaneStore.getState().activePaneId).toBe(ROOT_PANE_ID)
  })
})

describe('PaneContainer — the active-pane accent ring', () => {
  const paneBox = () => document.querySelector('[data-pane-content]') as HTMLElement
  const accent = () => document.querySelector('[data-pane-accent]') as HTMLElement

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

    expect(paneBox().style.borderTop).toBe('1px solid var(--pane-border)')

    await act(async () => {
      windowPaneStore.getState().paneActions.setActivePane(other)
    })

    expect(paneBox().style.borderTop).toBe('1px solid var(--pane-border)')
  })

  it('fades the overlay opacity between active and inactive, on the box’s own border geometry', async () => {
    const store = createWorkspaceStore('w1')
    windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })
    const other = splitSoTheRingApplies()
    windowPaneStore.getState().paneActions.setActivePane(ROOT_PANE_ID)

    await renderPane(store)

    expect(accent().style.borderTop).toBe('var(--pane-accent-width) solid var(--pane-accent)')
    expect(accent().style.opacity).toBe('1')
    expect(accent().style.left).toBe(paneBox().style.marginLeft)
    expect(accent().style.top).toBe(paneBox().style.marginTop)
    expect(accent().style.right).toBe(paneBox().style.marginRight)
    expect(accent().style.bottom).toBe(paneBox().style.marginBottom)
    expect(accent().style.borderTopLeftRadius).toBe(paneBox().style.borderTopLeftRadius)
    expect(accent().style.borderBottomRightRadius).toBe(paneBox().style.borderBottomRightRadius)
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
    setActiveWorkspaceId(null)
  })

  it('an unmeasured pane with the split on defaults to side by side: a divider, and the editor is not hidden', async () => {
    const store = createWorkspaceStore('w1')
    windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })
    seedEditorTab(store, ROOT_PANE_ID, 'tab-a')

    await renderPane(store)

    const chat = await screen.findByTestId('chat-chat-1')
    const editorMarker = await screen.findByTestId('editor-marker-tab-a')
    expect(chat.closest('[hidden]')).toBeNull()
    expect(editorMarker.closest('[hidden]')).toBeNull()
    expect(screen.getByRole('separator')).toHaveAttribute('aria-orientation', 'vertical')
  })

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

    expect(chatView.contains(screen.getByTestId('chat-branch-header'))).toBe(true)

    expect(editorView.style.borderLeft).toBe('1px solid var(--border)')
    expect(editorView.style.borderTopLeftRadius).toBe('0px')
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

      const outerRef = document.createElement('div')
      Object.assign(outerRef.style, buildPaneContentStyle(ROOT_PANE_POSITION, 'left', false, true))
      expect(editorView.style.borderTop).toBe('1px solid var(--border)')
      expect(editorView.style.borderTopLeftRadius).toBe('0px')
      expect(editorView.style.borderTopRightRadius).toBe('0px')
      expect(editorView.style.borderBottomLeftRadius).toBe(outerRef.style.borderBottomLeftRadius)
      expect(editorView.style.borderBottomRightRadius).toBe(outerRef.style.borderBottomRightRadius)
    })
  })

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

      expect(
        Boolean(editorView.compareDocumentPosition(chatView) & Node.DOCUMENT_POSITION_FOLLOWING),
      ).toBe(true)
      expect(editorView.style.borderRight).toBe('1px solid var(--border)')
      expect(editorView.style.borderTopRightRadius).toBe('0px')
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
      expect(editorView.style.borderTopLeftRadius).toBe('0px')
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
        expect(editorView.style.borderTopLeftRadius).toBe('0px')
        expect(editorView.style.borderTopRightRadius).toBe('0px')
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

  it('does not remount the editor view — including a live terminal — when a chat lands in the pane', async () => {
    terminalMountCount.current = 0
    const store = createWorkspaceStore('w1')
    seedTerminalTab(store, ROOT_PANE_ID, 'term-a')

    await renderPane(store)

    const terminalBefore = await screen.findByTestId('terminal-marker-term-a')
    expect(terminalMountCount.current).toBe(1)

    await act(async () => {
      windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })
    })

    await screen.findByTestId('chat-chat-1')
    expect(screen.getByTestId('terminal-marker-term-a')).toBe(terminalBefore)
    expect(terminalMountCount.current).toBe(1)
  })

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

    await act(async () => {
      windowPaneStore.getState().paneActions.activateEditorTabInPane(ROOT_PANE_ID, 'review-1')
    })

    await screen.findByTestId('branch-review-marker')
    expect(screen.getByTestId('editor-marker-tab-a')).toBe(editorBefore)
    expect(editorMountCount.current).toBe(1)
    expect((editorBefore.parentElement as HTMLElement).style.visibility).toBe('hidden')

    await act(async () => {
      windowPaneStore.getState().paneActions.activateEditorTabInPane(ROOT_PANE_ID, 'tab-a')
    })

    expect(screen.getByTestId('editor-marker-tab-a')).toBe(editorBefore)
    expect(editorMountCount.current).toBe(1)
    expect((editorBefore.parentElement as HTMLElement).style.visibility).not.toBe('hidden')
  })
})

describe('PaneContainer — pane drop target', () => {
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

  it('a drop of this pane’s own tab keeps one copy of it and never makes a split', async () => {
    const store = createWorkspaceStore('w1')
    seedEditorTab(store, ROOT_PANE_ID, 'existing-tab')
    seedEditorTab(store, ROOT_PANE_ID, 'other-tab')
    windowPaneStore.getState().paneActions.activateEditorTabInPane(ROOT_PANE_ID, 'other-tab')
    await renderPane(store)
    const paneCount = Object.keys(windowPaneStore.getState().panes).length

    centerDropPayload.current = { bufferId: 'existing-tab', paneId: ROOT_PANE_ID }
    await act(async () => {
      fireEvent.click(await screen.findByTestId('split-drop-trigger-center'))
    })

    const state = windowPaneStore.getState()
    expect(Object.keys(state.panes)).toHaveLength(paneCount)
    expect(state.panes[ROOT_PANE_ID].editorTabIds).toEqual(['existing-tab', 'other-tab'])
    centerDropPayload.current = { bufferId: 'existing-tab', paneId: 'phantom-source-pane' }
  })
})

describe("PaneContainer — the identity row shares the pane's background/rounding", () => {
  afterEach(() => {
    sidebarOpenOverride.current = true
    setActiveWorkspaceId(null)
  })

  it('nests the tab-bar row inside the same rounded/clipped box as the content — not an unstyled sibling of it', async () => {
    const store = createWorkspaceStore('w1')
    await renderPane(store)

    const sharedBox = document.querySelector('[data-pane-content]')!
    const row = screen.getByTestId('tab-bar-marker')
    expect(sharedBox.contains(row)).toBe(true)
    expect(sharedBox).toHaveClass('bg-pane-chrome-bg')

    const outer = document.querySelector('[data-pane-container]')!
    expect(outer.className).not.toMatch(/\bbg-/)
  })

  it("the shared box's rounding/border/gutter — not just data-pane-content alone — matches buildPaneContentStyle for the pane's actual edges (single pane, open left sidebar: right+bottom are real window edges)", async () => {
    const store = createWorkspaceStore('w1')
    await renderPane(store, ROOT_PANE_POSITION)

    const sharedBox = document.querySelector('[data-pane-content]')! as HTMLElement
    const expected = buildPaneContentStyle(ROOT_PANE_POSITION, 'left', false, true)
    const reference = document.createElement('div')
    Object.assign(reference.style, expected)
    expect(sharedBox.getAttribute('style')).toBe(reference.getAttribute('style'))

    expect(sharedBox.style.borderTopLeftRadius).toBe('var(--radius-lg)')
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

  it('still flattens the corner at a REAL window edge once the sidebar collapses — not just the common shielded case', async () => {
    sidebarOpenOverride.current = false
    const store = createWorkspaceStore('w1')
    await renderPane(store, ROOT_PANE_POSITION)

    const sharedBox = document.querySelector('[data-pane-content]')! as HTMLElement
    const expected = buildPaneContentStyle(ROOT_PANE_POSITION, 'left', false, false)
    expect(expected.borderTopLeftRadius).toBe('0')
    expect(expected.marginLeft).toBe('0')

    const reference = document.createElement('div')
    Object.assign(reference.style, expected)
    expect(sharedBox.getAttribute('style')).toBe(reference.getAttribute('style'))
    expect(sharedBox.style.borderTopLeftRadius).toBe('0px')
    expect(sharedBox.style.borderLeftStyle).toBe('none')
    expect(sharedBox.style.marginLeft).toBe('0px')

    const row = screen.getByTestId('tab-bar-marker')
    expect(sharedBox.contains(row)).toBe(true)
  })

  it("the shared box's active/inactive border swap is transitioned, not a same-frame snap", async () => {
    const store = createWorkspaceStore('w1')
    await renderPane(store)

    const sharedBox = document.querySelector('[data-pane-content]')!
    expect(sharedBox).toHaveClass('transition-colors')
  })
})

describe('PaneContainer — a pane in a view that is not on screen', () => {
  it('tells the chat surface it is not visible, and not the active pane', async () => {
    const store = createWorkspaceStore('w1')
    windowPaneStore.getState().paneActions.openChat('chat-1', { runnerId: 'runner-1' })

    await renderPane(store, undefined, false)

    const chat = screen.getByTestId('chat-chat-1')
    expect(chat.getAttribute('data-visible')).toBe('false')
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
    seedChats([chatRecord('chat-1', 'w-owner')])
    windowPaneStore
      .getState()
      .paneActions.openChat('chat-1', { runnerId: 'runner-1', workspaceId: 'w-owner' })

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

  it("shows the chat's own title in its header even while a DIFFERENT workspace is ambient", async () => {
    seedChats([chatRecord('chat-1', 'w-owner')])
    windowPaneStore
      .getState()
      .paneActions.openChat('chat-1', { runnerId: 'runner-1', workspaceId: 'w-owner' })

    await renderPane(createWorkspaceStore('w-onscreen'))

    expect(await screen.findByTestId('chat-branch-header')).toHaveTextContent('chat-1')
  })

  it("hands the editor area's actions the CHAT's own workspace, not the ambient one", async () => {
    seedChats([chatRecord('chat-1', 'w-owner')])
    windowPaneStore
      .getState()
      .paneActions.openChat('chat-1', { runnerId: 'runner-1', workspaceId: 'w-owner' })

    await renderPane(createWorkspaceStore('w-onscreen'))

    fireEvent.click(await screen.findByTestId('open-editor'))

    expect(await screen.findByTestId('new-tab-marker')).toHaveAttribute('data-ws-id', 'w-owner')
  })

  it("offers no editor actions while the chat's own owner is still unresolved", async () => {
    windowPaneStore.getState().paneActions.openChat('chat-unknown')

    await renderPane(createWorkspaceStore('w-onscreen'))

    await screen.findByTestId('chat-branch-header')
    expect(screen.queryByTestId('open-editor')).not.toBeInTheDocument()
    expect(screen.getByTestId('new-tab-marker')).toHaveAttribute('data-ws-id', '')
  })
})
