import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { ROOT_PANE_ID, BOTTOM_PANE_ID } from '@/features/panes/constants/pane'

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
const minted = [expect.any(String), expect.any(String)]

const createChat = vi.fn()
const toastSpawnFailure = vi.fn()
const presetChatLandingPresentation = vi.fn()
const getOwningChatId = vi.fn()
import * as createsApi from '@/features/agent/api/agent-chat-creates'
vi.spyOn(createsApi, 'createChat').mockImplementation(createChat)
import * as openDocumentApi from '@/features/editor/documents/open-document'
const openUntitled = vi.spyOn(openDocumentApi, 'openUntitledDocument').mockResolvedValue('')
import * as spawnError from '@/features/agent/lib/spawn-error'
vi.spyOn(spawnError, 'toastSpawnFailure').mockImplementation(toastSpawnFailure)
import * as chatPresentation from '@/features/agent/hooks/use-chat-presentation'
vi.spyOn(chatPresentation, 'presetChatLandingPresentation').mockImplementation(
  presetChatLandingPresentation,
)

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

vi.mock('@/features/panes/stores/window-pane-store', () => ({
  windowPaneStore: { getState: () => fakeState },
}))

import * as keymap from '@/features/keymaps/hooks/use-effective-keymap'
vi.spyOn(keymap, 'useEffectiveChordMap').mockImplementation(() => ({
  'tabs.closeActive': 'mod+w',
  'tabs.reopenClosed': 'mod+shift+t',
  'tabs.new': 'mod+t',
  'tabs.newTerminal': 'mod+j',
  'tabs.newFile': 'mod+shift+n',
  'agent.newChat': 'mod+n',
  'agent.newChatTerminal': 'mod+alt+n',
}))

import * as workspaceScope from '@/lib/workspace-scope'
vi.spyOn(workspaceScope, 'getOwningChatId').mockImplementation(getOwningChatId)

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
    expect(removeEditorTabFromPane).toHaveBeenCalledWith(ROOT_PANE_ID, 'buf-1')
    expect(closeBuffer).toHaveBeenCalledWith('buf-1')
  })

  it('prompts (pendingClose) instead of closing a DIRTY editor buffer', () => {
    fakeState.buffers = [{ id: 'buf-1', type: 'editor', isDirty: true }]
    renderHook(() => usePaneKeyboard())
    pressCmdW()
    expect(setPendingClose).toHaveBeenCalledWith({ bufferId: 'buf-1', paneId: ROOT_PANE_ID })
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

  it('mod+w on an empty pane in the LAST pane does nothing', () => {
    setPaneState({ activeEditorTabId: null, editorTabIds: [], paneCount: 1 })
    renderHook(() => usePaneKeyboard())
    pressCmdW()
    expect(closePane).not.toHaveBeenCalled()
    expect(removeEditorTabFromPane).not.toHaveBeenCalled()
  })

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

describe('usePaneKeyboard — agent.newChat chord', () => {
  const pressChord = () => press('n')

  it('creates a chat with the first enabled provider and opens it as its own new view', async () => {
    setProviders([cli('p1', 'Claude'), cli('p2', 'Codex')])
    createChat.mockResolvedValue('chat-9')
    renderHook(() => usePaneKeyboard())

    pressChord()
    expect(createChat).toHaveBeenCalledWith(...minted, 'ws-1', 'p1', '', undefined)

    await createChat.mock.results[0]?.value
    await Promise.resolve()

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

describe('usePaneKeyboard — agent.newChatTerminal chord', () => {
  const pressTerminalChord = () => press('n', { altKey: true })

  it('presets the new chat onto Terminal before opening it', async () => {
    setProviders([cli('p1', 'Claude', { hasTerminal: true, terminalStartHere: true })])
    createChat.mockResolvedValue('chat-9')
    renderHook(() => usePaneKeyboard())

    pressTerminalChord()
    expect(createChat).toHaveBeenCalledWith(...minted, 'ws-1', 'p1', '', 'terminal')

    await createChat.mock.results[0]?.value
    await Promise.resolve()

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

    expect(createChat).toHaveBeenCalledWith(...minted, 'ws-1', 'p1', '', undefined)
    expect(presetChatLandingPresentation).not.toHaveBeenCalled()
    expect(openChat).toHaveBeenCalledWith('chat-9', { workspaceId: 'ws-1' })
  })

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
