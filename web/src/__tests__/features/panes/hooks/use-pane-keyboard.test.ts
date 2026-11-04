import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { ROOT_PANE_ID, BOTTOM_PANE_ID } from '@/features/panes/constants/pane'

// Cmd+W must close the active tab, not fall through to the macOS menu's Window >
// Close and quit the app.
vi.mock('@/utils/platform', async () => {
  const actual = await vi.importActual<typeof import('@/utils/platform')>('@/utils/platform')
  return { ...actual, IS_MAC: true, APPLE_KEYBOARD: true }
})

const closeBuffer = vi.fn()
const reopenLastClosedBuffer = vi.fn()
const setPendingClose = vi.fn()
const removeEditorTabFromPane = vi.fn()
const navigateToPane = vi.fn()
const openContent = vi.fn()
const closePane = vi.fn()
const setActivePane = vi.fn()
const dropChatOnPane = vi.fn()
const openChat = vi.fn()
const minted = [expect.any(String), expect.any(String)] // chat id, operation id

// Hoisted: the mock factories below read these eagerly at factory-call time.
const { createChat, toastSpawnFailure, presetChatLandingPresentation } = vi.hoisted(() => ({
  createChat: vi.fn(),
  toastSpawnFailure: vi.fn(),
  presetChatLandingPresentation: vi.fn(),
}))
// providerCanStartOnTerminal runs for real: it is the gate under test below,
// so only createChat is faked.
import * as createsApi from '@/features/agent/api/agent-chat-creates'
vi.spyOn(createsApi, 'createChat').mockImplementation(createChat)
// The untitled document itself (its Monaco model, its free name) is open-document's own test.
import * as openDocumentApi from '@/features/editor/documents/open-document'
const openUntitled = vi.spyOn(openDocumentApi, 'openUntitledDocument').mockResolvedValue('')
vi.mock('@/features/agent/lib/spawn-error', () => ({ toastSpawnFailure }))
vi.mock('@/features/agent/hooks/use-chat-presentation', () => ({ presetChatLandingPresentation }))

type FakePane = {
  activeEditorTabId: string | null
  editorTabIds: string[]
  chatId?: string | null
  workspaceId?: string | null
}
type FakeLayout =
  | { type: 'pane'; id: string }
  | {
      type: 'split'
      id: string
      direction: 'horizontal' | 'vertical'
      sizes: [number, number]
      first: FakeLayout
      second: FakeLayout
    }

// `panes` always holds both ROOT_PANE_ID and BOTTOM_PANE_ID, as in production;
// "last remaining pane" is scoped per layout tree (`stage`, `bottomLayout`).
const fakeState = {
  activePaneId: ROOT_PANE_ID,
  workspaceId: 'ws-1',
  views: {},
  activeViewId: null,
  stage: { type: 'pane', id: ROOT_PANE_ID } as FakeLayout,
  bottomLayout: { type: 'pane', id: BOTTOM_PANE_ID } as FakeLayout,
  panes: {
    [ROOT_PANE_ID]: { activeEditorTabId: 'buf-1' as string | null, editorTabIds: ['buf-1'] },
    [BOTTOM_PANE_ID]: { activeEditorTabId: null as string | null, editorTabIds: [] },
  } as Record<string, FakePane>,
  buffers: [{ id: 'buf-1', type: 'editor', isDirty: false }] as Array<{
    id: string
    type: string
    isDirty?: boolean
  }>,

  bufferActions: {
    closeBuffer,
    reopenLastClosedBuffer,
    setPendingClose,
    openContent,
  },
  paneActions: {
    navigateToPane,
    removeEditorTabFromPane,
    closePane,
    setActivePane,
    dropChatOnPane,
    openChat,
  },
}
const fakeStore = { getState: () => fakeState }

vi.mock('@/features/workspace/stores/workspace-context', () => ({
  useWorkspaceStore: () => fakeStore,
}))

// Both stores read the same `fakeState`; the closure defers the read past hoisting.
vi.mock('@/features/panes/stores/window-pane-store', () => ({
  windowPaneStore: { getState: () => fakeState },
}))

vi.mock('@/features/keymaps/hooks/use-effective-keymap', () => ({
  useEffectiveChordMap: () => ({
    'tabs.closeActive': 'mod+w',
    'tabs.reopenClosed': 'mod+shift+t',
    'panes.splitRight': 'mod+\\',
    'tabs.new': 'mod+t',
    'tabs.newTerminal': 'mod+j',
    'tabs.newFile': 'mod+shift+n',
    'agent.newChat': 'mod+n',
    'agent.newChatTerminal': 'mod+alt+n',
  }),
}))

// ensurePaneChatThenOpen runs for REAL: a new terminal/file attaches the pane
// workspace's real owning chat (getOwningChatId, mocked below), never a new one.
vi.mock('@/features/panes/utils/pane-command-actions', async () => {
  const actual = await vi.importActual<
    typeof import('@/features/panes/utils/pane-command-actions')
  >('@/features/panes/utils/pane-command-actions')
  return { ...actual, splitActiveEditorGroup: vi.fn() }
})

const { getOwningChatId } = vi.hoisted(() => ({ getOwningChatId: vi.fn() }))
vi.mock('@/lib/workspace-scope', () => ({
  getOwningChatId,
}))

import { usePaneKeyboard } from '@/features/panes/hooks/use-pane-keyboard'
import { useSettingsStore } from '@/features/settings/store'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'

const setProviders = (providers: Array<Partial<AgentProvider>>) =>
  useAgentProvidersStore.setState({ status: 'ready', providers: providers as AgentProvider[] })
const cli = (id: string, displayName: string, over: Partial<AgentProvider> = {}) => ({
  ...{ id, displayName, icon: '', connected: true, enabled: true },
  ...over,
})
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { seedChats } from '@/__tests__/__fixtures__/agent-chat'

afterEach(() => setActiveWorkspaceId(null))

beforeEach(() => {
  vi.clearAllMocks()
  // The workspace in view is not the pane's: what a chord opens belongs to the pane's.
  setActiveWorkspaceId('ws-in-view')
  getOwningChatId.mockReset()
  fakeState.activePaneId = ROOT_PANE_ID
  setProviders([])
  fakeState.panes = {
    [ROOT_PANE_ID]: { activeEditorTabId: 'buf-1', editorTabIds: ['buf-1'] },
    [BOTTOM_PANE_ID]: { activeEditorTabId: null, editorTabIds: [] },
  }
  fakeState.stage = { type: 'pane', id: ROOT_PANE_ID }
  fakeState.bottomLayout = { type: 'pane', id: BOTTOM_PANE_ID }
  fakeState.buffers = [{ id: 'buf-1', type: 'editor', isDirty: false }]
})

const press = (key: string, init: KeyboardEventInit = {}) =>
  window.dispatchEvent(
    new KeyboardEvent('keydown', { key, metaKey: true, bubbles: true, cancelable: true, ...init }),
  )
const pressCmdW = () => press('w')

/** Sets the root pane's tabs and splits `stage` into `paneCount` leaves; the bottom
 *  pane stays its own single-leaf tree, never counted in the root split. */
function setPaneState({
  activeEditorTabId,
  editorTabIds,
  paneCount = 1,
}: {
  activeEditorTabId: string | null
  editorTabIds: string[]
  paneCount?: number
}) {
  const panes: Record<string, FakePane> = {
    [ROOT_PANE_ID]: { activeEditorTabId, editorTabIds },
    [BOTTOM_PANE_ID]: { activeEditorTabId: null, editorTabIds: [] },
  }
  let stage: FakeLayout = { type: 'pane', id: ROOT_PANE_ID }
  for (let i = 2; i <= paneCount; i++) {
    const extraId = `pane-${i}`
    panes[extraId] = { activeEditorTabId: null, editorTabIds: [] }
    stage = {
      type: 'split',
      id: `split-${i}`,
      direction: 'horizontal',
      sizes: [50, 50],
      first: stage,
      second: { type: 'pane', id: extraId },
    }
  }
  fakeState.panes = panes
  fakeState.stage = stage
  fakeState.bottomLayout = { type: 'pane', id: BOTTOM_PANE_ID }
}

/** Splits the BOTTOM panel itself into `paneCount` leaves (independent of the
 *  root tree), for the "sensibly handle the bottom pane" cases. */
function setBottomPaneState({
  activeEditorTabId,
  editorTabIds,
  paneCount = 1,
}: {
  activeEditorTabId: string | null
  editorTabIds: string[]
  paneCount?: number
}) {
  const panes: Record<string, FakePane> = {
    [ROOT_PANE_ID]: { activeEditorTabId: null, editorTabIds: [] },
    [BOTTOM_PANE_ID]: { activeEditorTabId, editorTabIds },
  }
  let bottomLayout: FakeLayout = { type: 'pane', id: BOTTOM_PANE_ID }
  for (let i = 2; i <= paneCount; i++) {
    const extraId = `bottom-pane-${i}`
    panes[extraId] = { activeEditorTabId: null, editorTabIds: [] }
    bottomLayout = {
      type: 'split',
      id: `bottom-split-${i}`,
      direction: 'horizontal',
      sizes: [50, 50],
      first: bottomLayout,
      second: { type: 'pane', id: extraId },
    }
  }
  fakeState.panes = panes
  fakeState.stage = { type: 'pane', id: ROOT_PANE_ID }
  fakeState.bottomLayout = bottomLayout
}

describe('usePaneKeyboard — Cmd+W closes the active tab', () => {
  it('removes the active tab from its pane (so a neighbor activates) AND closes it', () => {
    renderHook(() => usePaneKeyboard())
    pressCmdW()
    // removeEditorTabFromPane is what activates the adjacent tab — without it
    // the pane is left with a dangling activeEditorTabId and falls to the
    // empty state.
    expect(removeEditorTabFromPane).toHaveBeenCalledWith(ROOT_PANE_ID, 'buf-1')
    expect(closeBuffer).toHaveBeenCalledWith('buf-1')
  })

  it('prompts (pendingClose) instead of closing a DIRTY editor buffer', () => {
    fakeState.buffers = [{ id: 'buf-1', type: 'editor', isDirty: true }]
    renderHook(() => usePaneKeyboard())
    pressCmdW()
    expect(setPendingClose).toHaveBeenCalledWith({ type: 'single', bufferId: 'buf-1' })
    expect(closeBuffer).not.toHaveBeenCalled()
    expect(removeEditorTabFromPane).not.toHaveBeenCalled()
  })

  it('does nothing on mod+w when there is no active tab', () => {
    setPaneState({ activeEditorTabId: null, editorTabIds: [] })
    renderHook(() => usePaneKeyboard())
    pressCmdW()
    expect(closeBuffer).not.toHaveBeenCalled()
    expect(removeEditorTabFromPane).not.toHaveBeenCalled()
  })
})

// ⌘W on a pane with no tabs dismisses it in a split and is a no-op in the last one.
describe('usePaneKeyboard — mod+w on a pane with no tabs', () => {
  it('mod+w on an empty pane in a split closes the split pane', () => {
    setPaneState({ activeEditorTabId: null, editorTabIds: [], paneCount: 2 })
    renderHook(() => usePaneKeyboard())
    pressCmdW()
    expect(closePane).toHaveBeenCalledWith(ROOT_PANE_ID)
    expect(removeEditorTabFromPane).not.toHaveBeenCalled()
  })

  it('mod+w on a split pane whose chat is mid-turn just closes the view of it', () => {
    setPaneState({ activeEditorTabId: null, editorTabIds: [], paneCount: 2 })
    fakeState.panes[ROOT_PANE_ID] = {
      ...fakeState.panes[ROOT_PANE_ID],
      chatId: 'busy-chat',
      workspaceId: 'ws-1',
    } as FakePane
    seedChats([{ id: 'busy-chat', workspaceId: 'ws-1', working: true }])
    renderHook(() => usePaneKeyboard())
    pressCmdW()
    expect(closePane).toHaveBeenCalledWith(ROOT_PANE_ID)
  })

  // Counting `panes` (which holds the bottom pane too) closed the only editor
  // pane and bricked the workspace; its own tree has one leaf.
  it('mod+w on an empty pane in the LAST pane does nothing', () => {
    setPaneState({ activeEditorTabId: null, editorTabIds: [], paneCount: 1 })
    renderHook(() => usePaneKeyboard())
    pressCmdW()
    expect(closePane).not.toHaveBeenCalled()
    expect(removeEditorTabFromPane).not.toHaveBeenCalled()
  })

  // The two trees are scoped independently: a root split does not count here.
  it('mod+w on the ONLY (empty) bottom pane does nothing, even with a split root', () => {
    setPaneState({ activeEditorTabId: 'buf-1', editorTabIds: ['buf-1'], paneCount: 2 })
    fakeState.activePaneId = BOTTOM_PANE_ID
    fakeState.panes[BOTTOM_PANE_ID] = { activeEditorTabId: null, editorTabIds: [] }
    renderHook(() => usePaneKeyboard())
    pressCmdW()
    expect(closePane).not.toHaveBeenCalled()
    expect(removeEditorTabFromPane).not.toHaveBeenCalled()
  })

  it('mod+w on an empty pane in a SPLIT bottom panel closes that bottom split', () => {
    fakeState.activePaneId = BOTTOM_PANE_ID
    setBottomPaneState({ activeEditorTabId: null, editorTabIds: [], paneCount: 2 })
    renderHook(() => usePaneKeyboard())
    pressCmdW()
    expect(closePane).toHaveBeenCalledWith(BOTTOM_PANE_ID)
    expect(removeEditorTabFromPane).not.toHaveBeenCalled()
  })
})

describe('usePaneKeyboard — new tab / terminal / file chords', () => {
  it('mod+t opens a New Tab page in the active pane', () => {
    fakeState.panes[ROOT_PANE_ID].chatId = 'chat-1'
    renderHook(() => usePaneKeyboard())
    press('t')
    expect(openContent).toHaveBeenCalledWith(
      { type: 'newTabPage', workspaceId: 'ws-1' },
      { paneId: ROOT_PANE_ID },
    )
    expect(createChat).not.toHaveBeenCalled()
  })

  describe('mod+n with focus inside an editor area', () => {
    let host: HTMLElement
    beforeEach(() => {
      fakeState.panes['pane-2'] = { activeEditorTabId: null, editorTabIds: [], chatId: 'chat-2' }
      setProviders([cli('p1', 'Claude')])
      createChat.mockResolvedValue('chat-9')
      host = document.createElement('div')
      host.innerHTML =
        '<div data-pane-id="pane-2"><div data-editor-view=""><button>in editor</button></div>' +
        '<button>in chat</button></div>'
      document.body.appendChild(host)
    })
    afterEach(() => host.remove())

    it('opens a New Tab page in THAT pane instead of a new chat', () => {
      host.querySelector<HTMLButtonElement>('[data-editor-view] button')!.focus()
      renderHook(() => usePaneKeyboard())
      press('n')
      expect(openContent).toHaveBeenCalledWith(
        { type: 'newTabPage', workspaceId: 'ws-1' },
        { paneId: 'pane-2' },
      )
      expect(createChat).not.toHaveBeenCalled()
    })

    it('stays New Chat when focus is in the pane but outside its editor area', () => {
      host.querySelectorAll<HTMLButtonElement>('button')[1]!.focus()
      renderHook(() => usePaneKeyboard())
      press('n')
      expect(createChat).toHaveBeenCalled()
      expect(openContent).not.toHaveBeenCalled()
    })
  })

  it("mod+j opens a terminal, attaching the workspace's real owning chat", () => {
    // The active pane has no chat yet, so ensurePaneChatThenOpen must attach
    // the workspace's real owning chat first (Law 3) — never mint one.
    getOwningChatId.mockReturnValue('chat-1')
    renderHook(() => usePaneKeyboard())
    press('j')

    expect(getOwningChatId).toHaveBeenCalledWith('ws-1')
    expect(dropChatOnPane).toHaveBeenCalledWith('chat-1', ROOT_PANE_ID, 'center', 'ws-1')
    expect(openContent.mock.calls[0]?.[0]).toEqual({ type: 'terminal', workspaceId: 'ws-1' })
    expect(openContent.mock.calls[0]?.[1]).toEqual({ paneId: ROOT_PANE_ID })
    expect(createChat).not.toHaveBeenCalled()
  })

  it('mod+j does nothing — no chat attached, no terminal opened — when no owning chat resolves', () => {
    getOwningChatId.mockReturnValue(null)
    renderHook(() => usePaneKeyboard())
    press('j')

    expect(dropChatOnPane).not.toHaveBeenCalled()
    expect(openChat).not.toHaveBeenCalled()
    expect(openContent).not.toHaveBeenCalled()
    expect(createChat).not.toHaveBeenCalled()
  })

  it("mod+shift+n opens an untitled virtual buffer, attaching the workspace's real owning chat", () => {
    getOwningChatId.mockReturnValue('chat-1')
    renderHook(() => usePaneKeyboard())
    press('n', { shiftKey: true })

    expect(dropChatOnPane).toHaveBeenCalledWith('chat-1', ROOT_PANE_ID, 'center', 'ws-1')
    expect(openUntitled).toHaveBeenCalledWith('ws-1', ROOT_PANE_ID)
    expect(createChat).not.toHaveBeenCalled()
  })
})

describe('usePaneKeyboard — agent.newChat chord (I4)', () => {
  const pressChord = () => press('n')

  it('creates a chat with the first enabled provider and opens it as its own new view', async () => {
    setProviders([cli('p1', 'Claude'), cli('p2', 'Codex')])
    createChat.mockResolvedValue('chat-9')
    renderHook(() => usePaneKeyboard())

    pressChord()
    // Picks the FIRST ENABLED provider without asking and names no surface: a
    // plain new chat lands on the provider's own default face.
    expect(createChat).toHaveBeenCalledWith(...minted, 'ws-1', 'p1', '', undefined)

    await createChat.mock.results[0]?.value
    await Promise.resolve()

    // ⌘N opens the chat the way a click does — its own row — never into
    // whatever the active pane is showing.
    expect(openChat).toHaveBeenCalledWith('chat-9', { workspaceId: 'ws-1' })
    expect(dropChatOnPane).not.toHaveBeenCalled()
    expect(openContent).not.toHaveBeenCalled()
  })

  it('does nothing when no provider is available (no CLI installed)', () => {
    setProviders([])
    renderHook(() => usePaneKeyboard())
    pressChord()
    expect(createChat).not.toHaveBeenCalled()
    expect(dropChatOnPane).not.toHaveBeenCalled()
    expect(openChat).not.toHaveBeenCalled()
  })

  it('picks the first ENABLED provider, skipping a disabled leading one', () => {
    // p1 disabled, p2 enabled → the chord must open p2, never the disabled p1.
    setProviders([cli('p1', 'Claude', { enabled: false }), cli('p2', 'Codex')])
    createChat.mockResolvedValue('chat-9')
    renderHook(() => usePaneKeyboard())
    pressChord()
    expect(createChat).toHaveBeenCalledWith(...minted, 'ws-1', 'p2', '', undefined)
  })

  it('does nothing when every provider is disabled', () => {
    setProviders([cli('p1', 'Claude', { enabled: false })])
    renderHook(() => usePaneKeyboard())
    pressChord()
    expect(createChat).not.toHaveBeenCalled()
    expect(dropChatOnPane).not.toHaveBeenCalled()
    expect(openChat).not.toHaveBeenCalled()
  })

  it('reports a spawn failure via toast instead of swallowing it', async () => {
    setProviders([cli('p1', 'Claude')])
    const err = new Error('boom')
    createChat.mockRejectedValue(err)
    renderHook(() => usePaneKeyboard())

    pressChord()
    await createChat.mock.results[0]?.value.catch(() => {})
    await Promise.resolve()

    expect(toastSpawnFailure).toHaveBeenCalledWith(err, 'Claude', 'start')
  })
})

// agent.newChatTerminal is agent.newChat landing on Terminal: a per-chat choice
// that leaves the global default untouched.
describe('usePaneKeyboard — agent.newChatTerminal chord', () => {
  const pressTerminalChord = () => press('n', { altKey: true })

  it('presets the new chat onto Terminal before opening it', async () => {
    setProviders([cli('p1', 'Claude', { hasTerminal: true, terminalStartHere: true })])
    createChat.mockResolvedValue('chat-9')
    renderHook(() => usePaneKeyboard())

    pressTerminalChord()
    // The surface rides the create: it decides which face the daemon forks.
    expect(createChat).toHaveBeenCalledWith(...minted, 'ws-1', 'p1', '', 'terminal')

    await createChat.mock.results[0]?.value
    await Promise.resolve()

    // Preset BEFORE the pane opens, or a pane mounted off the same microtask
    // queue could seed from the global default first.
    expect(presetChatLandingPresentation).toHaveBeenCalledWith('chat-9', 'terminal')
    expect(openChat).toHaveBeenCalledWith('chat-9', { workspaceId: 'ws-1' })
  })

  it('does not preset Terminal for a provider with no terminal at all (absence, not a disabled control)', async () => {
    setProviders([cli('p1', 'Claude', { hasTerminal: false })])
    createChat.mockResolvedValue('chat-9')
    renderHook(() => usePaneKeyboard())

    pressTerminalChord()
    await createChat.mock.results[0]?.value
    await Promise.resolve()

    // Still an ordinary create — same as plain agent.newChat — just never
    // told to land somewhere this provider cannot show.
    expect(createChat).toHaveBeenCalledWith(...minted, 'ws-1', 'p1', '', undefined)
    expect(presetChatLandingPresentation).not.toHaveBeenCalled()
    expect(openChat).toHaveBeenCalledWith('chat-9', { workspaceId: 'ws-1' })
  })

  // hasTerminal alone is not a landing surface: a terminal reached only by
  // switching after a turn lacks terminalStartHere.
  it('does not preset Terminal for a provider whose terminal is not a start_here surface, even though it has one', async () => {
    setProviders([cli('p1', 'Codex', { hasTerminal: true })])
    createChat.mockResolvedValue('chat-9')
    renderHook(() => usePaneKeyboard())

    pressTerminalChord()
    await createChat.mock.results[0]?.value
    await Promise.resolve()

    expect(presetChatLandingPresentation).not.toHaveBeenCalled()
    expect(openChat).toHaveBeenCalledWith('chat-9', { workspaceId: 'ws-1' })
  })

  it('plain agent.newChat never presets a surface', async () => {
    setProviders([cli('p1', 'Claude')])
    createChat.mockResolvedValue('chat-9')
    renderHook(() => usePaneKeyboard())

    press('n')
    await createChat.mock.results[0]?.value
    await Promise.resolve()

    expect(presetChatLandingPresentation).not.toHaveBeenCalled()
  })
})

// With Terminal as the default landing surface the plain chord must name it on the
// create too, or the daemon forks a face with no PTY to land on.
describe('usePaneKeyboard — agent.newChat honours the default landing surface', () => {
  const pressChord = () => press('n')
  function landingDefaultIsTerminal() {
    useSettingsStore.setState((state) => ({
      settings: { ...state.settings, chatIsDefaultPresentation: false },
    }))
  }

  afterEach(() => {
    useSettingsStore.setState((state) => ({
      settings: { ...state.settings, chatIsDefaultPresentation: true },
    }))
  })

  it('creates the chat ON the terminal when that is the default and the provider may start there', async () => {
    landingDefaultIsTerminal()
    setProviders([cli('codex', 'Codex', { hasTerminal: true, terminalStartHere: true })])
    createChat.mockResolvedValue('chat-9')
    renderHook(() => usePaneKeyboard())

    pressChord()
    expect(createChat).toHaveBeenCalledWith(...minted, 'ws-1', 'codex', '', 'terminal')

    await createChat.mock.results[0]?.value
    await Promise.resolve()
    // Created on the terminal ⇒ lands on it: one invariant, not two facts that
    // happen to agree while the preference sits where it does.
    expect(presetChatLandingPresentation).toHaveBeenCalledWith('chat-9', 'terminal')
  })

  it('still creates exactly as before for a provider whose terminal is no landing surface', async () => {
    landingDefaultIsTerminal()
    setProviders([cli('p1', 'Claude', { hasTerminal: true })])
    createChat.mockResolvedValue('chat-9')
    renderHook(() => usePaneKeyboard())

    pressChord()
    expect(createChat).toHaveBeenCalledWith(...minted, 'ws-1', 'p1', '', undefined)

    await createChat.mock.results[0]?.value
    await Promise.resolve()
    expect(presetChatLandingPresentation).not.toHaveBeenCalled()
  })
})
