import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { ROOT_PANE_ID, BOTTOM_PANE_ID } from '@/features/panes/constants/pane'

// Cmd+W must close the ACTIVE tab — not the app. Previously NO close-tab command
// existed, so on desktop Cmd+W fell through to the macOS default menu's
// Window > Close and quit Crowbar. This locks the registry binding -> the
// use-pane-keyboard dispatch that closes the active buffer instead.
vi.mock('@/utils/platform', async () => {
  const actual = await vi.importActual<typeof import('@/utils/platform')>('@/utils/platform')
  return { ...actual, IS_MAC: true }
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
const setActiveAgentChatId = vi.fn()

// I4: the AGENT_NEW_CHAT chord (⌘N) creates a chat via the agent API — mocked
// here the same way NewTabView's own regression tests mock it. `vi.hoisted` is
// required (not a plain const): the mock factory below reads `createChat`
// directly (an eager shorthand-property read at factory-call time, unlike the
// `() => fakeStore` closures elsewhere in this file, which defer the read).
const { createChat, toastSpawnFailure, presetChatLandingPresentation } = vi.hoisted(() => ({
  createChat: vi.fn(),
  toastSpawnFailure: vi.fn(),
  presetChatLandingPresentation: vi.fn(),
}))
// providerCanStartOnTerminal runs for REAL (vi.importActual): it is the
// gate under test in the cases below, so a stub would make those assertions
// meaningless — only createChat/getPendingPrompt are faked.
vi.mock('@/features/agent/api/agent-api', async () => {
  const actual = await vi.importActual<typeof import('@/features/agent/api/agent-api')>(
    '@/features/agent/api/agent-api',
  )
  return {
    ...actual,
    getPendingPrompt: vi.fn().mockResolvedValue(null),
    createChat,
  }
})
vi.mock('@/features/agent/lib/spawn-error', () => ({ toastSpawnFailure }))
vi.mock('@/features/agent/hooks/use-chat-presentation', () => ({ presetChatLandingPresentation }))

type FakePane = { activeEditorTabId: string | null; editorTabIds: string[]; chatId?: string | null }
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

// `state.panes` in production ALWAYS holds BOTH ROOT_PANE_ID and BOTTOM_PANE_ID
// (see pane-slice.ts's initial state) — there is no such thing as a workspace
// where `panes` has a single entry. `stage`/`bottomLayout` are the two
// independent layout trees getPaneScopeForPaneId scopes "last remaining pane"
// against (C1: a raw `Object.keys(state.panes).length` conflates the two
// trees and is never 1, even in a genuinely single-pane workspace).
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
  agentChats: {
    providers: [] as Array<{
      id: string
      displayName: string
      icon: string
      connected?: boolean
      enabled?: boolean
      hasTerminal?: boolean
      terminalStartHere?: boolean
    }>,
    chats: [] as Array<{ id: string; title: string }>,
  },
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
  setActiveAgentChatId,
}
const fakeStore = { getState: () => fakeState }

vi.mock('@/features/workspace/stores/workspace-context', () => ({
  useWorkspaceStore: () => fakeStore,
}))

// Task 26: panes/buffers moved off the per-workspace store onto the
// window-level singleton — usePaneKeyboard reads activePaneId/panes/buffers/
// paneActions/bufferActions off `windowPaneStore` now, and only agentChats/
// workspaceId/setActiveAgentChatId off `useWorkspaceStore()`. Both mocks point
// at the SAME `fakeState` object (it already carries every field either read
// site needs), so the `setPaneState`/`setBottomPaneState` helpers below that
// mutate `fakeState.panes` etc. in place are observed by both.
vi.mock('@/features/panes/stores/window-pane-store', () => ({
  // Not a direct `fakeStore` reference: `vi.mock` factories run at import-
  // hoist time, before this file's own `const fakeStore = ...` below has
  // executed. Wrapping the read in a closure (mirroring `useWorkspaceStore`'s
  // `() => fakeStore` just above) defers it until `getState()` is actually
  // called, by which point `fakeState` exists.
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

// ensurePaneChatThenOpen (the Law 3 fix — TAB_NEW_TERMINAL/TAB_NEW_FILE must
// attach a pane's WORKSPACE's real owning chat before opening a terminal/file
// into it, never mint a redundant new one) runs for REAL here. Its one real
// dependency, getOwningChatId, is mocked below; `getOrCreateWorkspaceStore`/
// createChat are no longer on this path at all — see pane-command-actions.ts.
// Only splitActiveEditorGroup stays a bare stub — it is unrelated to this fix.
vi.mock('@/features/panes/utils/pane-command-actions', async () => {
  const actual = await vi.importActual<
    typeof import('@/features/panes/utils/pane-command-actions')
  >('@/features/panes/utils/pane-command-actions')
  return { ...actual, splitActiveEditorGroup: vi.fn() }
})

const { getOwningChatId } = vi.hoisted(() => ({ getOwningChatId: vi.fn() }))
vi.mock('@/lib/workspace-scope', () => ({ getOwningChatId }))

// pane-command-actions.ts (run for real above) also imports getActiveWorkspaceId
// from the registry (for openBranchReviewForActiveWorkspace, unexercised here) —
// mocked so importing it stays cheap: the real module pulls in the editor/Monaco
// store graph, which previously timed out this suite's dynamic imports.
vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getActiveWorkspaceId: () => null,
}))

import { usePaneKeyboard } from '@/features/panes/hooks/use-pane-keyboard'

beforeEach(() => {
  vi.clearAllMocks()
  getOwningChatId.mockReset()
  fakeState.activePaneId = ROOT_PANE_ID
  fakeState.agentChats = { providers: [], chats: [] }
  fakeState.panes = {
    [ROOT_PANE_ID]: { activeEditorTabId: 'buf-1', editorTabIds: ['buf-1'] },
    [BOTTOM_PANE_ID]: { activeEditorTabId: null, editorTabIds: [] },
  }
  fakeState.stage = { type: 'pane', id: ROOT_PANE_ID }
  fakeState.bottomLayout = { type: 'pane', id: BOTTOM_PANE_ID }
  fakeState.buffers = [{ id: 'buf-1', type: 'editor', isDirty: false }]
})

function pressCmdW() {
  window.dispatchEvent(
    new KeyboardEvent('keydown', { key: 'w', metaKey: true, bubbles: true, cancelable: true }),
  )
}

/**
 * Sets the ROOT_PANE_ID group's active buffer/tabs and, when `paneCount > 1`,
 * splits `stage` into `paneCount` root-tree leaves (padding out extra,
 * otherwise-empty panes) so getPaneScopeForPaneId's root-scoped count reflects
 * a real split. BOTTOM_PANE_ID is always present as its own single-leaf tree,
 * exactly like a real workspace's ever-present bottom panel — it must never be
 * counted as part of the root split.
 */
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

// A pane with zero editorTabIds shows its own empty stage for free — there is
// no more placeholder 'newTab' buffer to special-case. ⌘W on such a pane must
// still behave the same way the old sole-New-Tab-buffer handling did: dismiss
// the pane in a split, no-op in the last remaining one.
describe('usePaneKeyboard — mod+w on a pane with no tabs', () => {
  it('mod+w on an empty pane in a split closes the split pane', () => {
    setPaneState({ activeEditorTabId: null, editorTabIds: [], paneCount: 2 })
    renderHook(() => usePaneKeyboard())
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'w', metaKey: true }))
    expect(closePane).toHaveBeenCalledWith(ROOT_PANE_ID)
    expect(removeEditorTabFromPane).not.toHaveBeenCalled()
  })

  // C1 regression: `state.panes` ALSO holds BOTTOM_PANE_ID here (this is the
  // real, single-pane-workspace shape — see the fixture comment above), so the
  // old `Object.keys(state.panes).length > 1` guard was always true and called
  // closePane on the workspace's ONLY editor pane, which reseeds and then
  // immediately deletes it again in pane-slice (bricking the workspace). Scoped
  // correctly, ROOT_PANE_ID's own tree has exactly one leaf, so this must no-op.
  it('mod+w on an empty pane in the LAST pane does nothing', () => {
    setPaneState({ activeEditorTabId: null, editorTabIds: [], paneCount: 1 })
    renderHook(() => usePaneKeyboard())
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'w', metaKey: true }))
    expect(closePane).not.toHaveBeenCalled()
    expect(removeEditorTabFromPane).not.toHaveBeenCalled()
  })

  // "Handle the bottom pane sensibly too": an empty bottom panel (single, un-
  // split) must no-op exactly like the sole root pane, regardless of how many
  // editor panes exist in the root tree — the two trees are scoped
  // independently.
  it('mod+w on the ONLY (empty) bottom pane does nothing, even with a split root', () => {
    setPaneState({ activeEditorTabId: 'buf-1', editorTabIds: ['buf-1'], paneCount: 2 })
    fakeState.activePaneId = BOTTOM_PANE_ID
    fakeState.panes[BOTTOM_PANE_ID] = { activeEditorTabId: null, editorTabIds: [] }
    renderHook(() => usePaneKeyboard())
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'w', metaKey: true }))
    expect(closePane).not.toHaveBeenCalled()
    expect(removeEditorTabFromPane).not.toHaveBeenCalled()
  })

  it('mod+w on an empty pane in a SPLIT bottom panel closes that bottom split', () => {
    fakeState.activePaneId = BOTTOM_PANE_ID
    setBottomPaneState({ activeEditorTabId: null, editorTabIds: [], paneCount: 2 })
    renderHook(() => usePaneKeyboard())
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'w', metaKey: true }))
    expect(closePane).toHaveBeenCalledWith(BOTTOM_PANE_ID)
    expect(removeEditorTabFromPane).not.toHaveBeenCalled()
  })
})

describe('usePaneKeyboard — new tab / terminal / file chords', () => {
  // A New Tab is no longer a mintable placeholder tab — a pane already shows
  // its own empty stage for free whenever it holds no editor tabs, and there
  // is no primitive yet for "detach the active tab without closing it" to
  // reproduce the old "add a blank scratch tab beside my real ones" gesture.
  // The chord is inert until one exists; it must not fall through and open a
  // terminal or anything else.
  it('mod+t is currently a no-op (no more mintable New Tab placeholder)', () => {
    renderHook(() => usePaneKeyboard())
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 't', metaKey: true }))
    expect(openContent).not.toHaveBeenCalled()
    expect(dropChatOnPane).not.toHaveBeenCalled()
    expect(openChat).not.toHaveBeenCalled()
  })

  it("mod+j opens a terminal, attaching the workspace's real owning chat", () => {
    // The active pane has no chat yet, so ensurePaneChatThenOpen must attach
    // the workspace's real owning chat first (Law 3) — never mint one.
    getOwningChatId.mockReturnValue('chat-1')
    renderHook(() => usePaneKeyboard())
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'j', metaKey: true }))

    expect(getOwningChatId).toHaveBeenCalledWith('ws-1')
    expect(dropChatOnPane).toHaveBeenCalledWith('chat-1', ROOT_PANE_ID, 'center')
    expect(openContent).toHaveBeenCalledWith({ type: 'terminal' })
    expect(createChat).not.toHaveBeenCalled()
  })

  it('mod+j does nothing — no chat attached, no terminal opened — when no owning chat resolves', () => {
    getOwningChatId.mockReturnValue(null)
    renderHook(() => usePaneKeyboard())
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'j', metaKey: true }))

    expect(dropChatOnPane).not.toHaveBeenCalled()
    expect(openChat).not.toHaveBeenCalled()
    expect(openContent).not.toHaveBeenCalled()
    expect(createChat).not.toHaveBeenCalled()
  })

  it("mod+shift+n opens an untitled virtual buffer, attaching the workspace's real owning chat", () => {
    getOwningChatId.mockReturnValue('chat-1')
    renderHook(() => usePaneKeyboard())
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'n', metaKey: true, shiftKey: true }))

    expect(dropChatOnPane).toHaveBeenCalledWith('chat-1', ROOT_PANE_ID, 'center')
    expect(openContent).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'editor', isVirtual: true }),
    )
    expect(createChat).not.toHaveBeenCalled()
  })

  // The two N chords are dispatched by the same handler in registry order, so
  // the unshifted one must NOT fall through to New File.
  it('mod+n does not open a file — that chord is New Chat now', () => {
    renderHook(() => usePaneKeyboard())
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'n', metaKey: true }))
    expect(openContent).not.toHaveBeenCalledWith(
      expect.objectContaining({ type: 'editor', isVirtual: true }),
    )
  })
})

// I4: the AGENT_NEW_CHAT chord (now mod+n) was registered in the keymap and
// shown as a badge on the New Tab surface, but nothing dispatched it — pressing
// it did nothing at all.
describe('usePaneKeyboard — agent.newChat chord (I4)', () => {
  function pressChord() {
    window.dispatchEvent(
      new KeyboardEvent('keydown', {
        key: 'n',
        metaKey: true,
        bubbles: true,
        cancelable: true,
      }),
    )
  }

  it('creates a chat with the first enabled provider and opens it as its own new view', async () => {
    fakeState.agentChats = {
      providers: [
        { id: 'p1', displayName: 'Claude', icon: '', connected: true, enabled: true },
        { id: 'p2', displayName: 'Codex', icon: '', connected: true, enabled: true },
      ],
      chats: [],
    }
    createChat.mockResolvedValue('chat-9')
    renderHook(() => usePaneKeyboard())

    pressChord()
    // Provider-agnostic: picks the FIRST ENABLED provider without asking, and
    // names no surface — a plain new chat lands on the provider's own default
    // face, which is what the daemon then forks.
    expect(createChat).toHaveBeenCalledWith('ws-1', 'p1', '', undefined)

    fakeState.agentChats.chats = [{ id: 'chat-9', title: 'New conversation' }]
    await createChat.mock.results[0]?.value
    await Promise.resolve()

    expect(setActiveAgentChatId).toHaveBeenCalledWith('chat-9')
    // ⌘N opens the chat the way a click does — its own row — never into
    // whatever the active pane is showing.
    expect(openChat).toHaveBeenCalledWith('chat-9')
    expect(dropChatOnPane).not.toHaveBeenCalled()
    expect(openContent).not.toHaveBeenCalled()
  })

  it('does nothing when no provider is available (no CLI installed)', () => {
    fakeState.agentChats = { providers: [], chats: [] }
    renderHook(() => usePaneKeyboard())
    pressChord()
    expect(createChat).not.toHaveBeenCalled()
    expect(dropChatOnPane).not.toHaveBeenCalled()
    expect(openChat).not.toHaveBeenCalled()
  })

  it('picks the first ENABLED provider, skipping a disabled leading one', () => {
    // p1 disabled, p2 enabled → the chord must open p2, never the disabled p1.
    fakeState.agentChats = {
      providers: [
        { id: 'p1', displayName: 'Claude', icon: '', connected: true, enabled: false },
        { id: 'p2', displayName: 'Codex', icon: '', connected: true, enabled: true },
      ],
      chats: [],
    }
    createChat.mockResolvedValue('chat-9')
    renderHook(() => usePaneKeyboard())
    pressChord()
    expect(createChat).toHaveBeenCalledWith('ws-1', 'p2', '', undefined)
  })

  it('does nothing when every provider is disabled', () => {
    fakeState.agentChats = {
      providers: [{ id: 'p1', displayName: 'Claude', icon: '', connected: true, enabled: false }],
      chats: [],
    }
    renderHook(() => usePaneKeyboard())
    pressChord()
    expect(createChat).not.toHaveBeenCalled()
    expect(dropChatOnPane).not.toHaveBeenCalled()
    expect(openChat).not.toHaveBeenCalled()
  })

  it('reports a spawn failure via toast instead of swallowing it', async () => {
    fakeState.agentChats = {
      providers: [{ id: 'p1', displayName: 'Claude', icon: '', connected: true, enabled: true }],
      chats: [],
    }
    const err = new Error('boom')
    createChat.mockRejectedValue(err)
    renderHook(() => usePaneKeyboard())

    pressChord()
    await createChat.mock.results[0]?.value.catch(() => {})
    await Promise.resolve()

    expect(toastSpawnFailure).toHaveBeenCalledWith(err, 'Claude', 'start')
  })
})

// THE BUG: "can't start chats directly on a CLI, it always obligates me to
// use the native chat" — no creation entry point ever asked for a landing
// surface, they all just accepted whatever chatIsDefaultPresentation said.
// agent.newChatTerminal is the same create as agent.newChat, but it presets
// the new chat's landing surface to Terminal first — a per-chat choice that
// leaves the global default (and every OTHER new chat) untouched.
describe('usePaneKeyboard — agent.newChatTerminal chord', () => {
  function pressTerminalChord() {
    window.dispatchEvent(
      new KeyboardEvent('keydown', {
        key: 'n',
        metaKey: true,
        altKey: true,
        bubbles: true,
        cancelable: true,
      }),
    )
  }

  it('presets the new chat onto Terminal before opening it', async () => {
    fakeState.agentChats = {
      providers: [
        {
          id: 'p1',
          displayName: 'Claude',
          icon: '',
          connected: true,
          enabled: true,
          hasTerminal: true,
          terminalStartHere: true,
        },
      ],
      chats: [],
    }
    createChat.mockResolvedValue('chat-9')
    renderHook(() => usePaneKeyboard())

    pressTerminalChord()
    // The surface rides the CREATE, not just the landing seed: it is what
    // decides which of the provider's faces the daemon actually forks. A chat
    // seeded onto Terminal but spawned on the api transport is the "no
    // terminal view attached right now" the user hit.
    expect(createChat).toHaveBeenCalledWith('ws-1', 'p1', '', 'terminal')

    await createChat.mock.results[0]?.value
    await Promise.resolve()

    // Preset BEFORE the pane opens, or a pane mounted off the same microtask
    // queue could seed from the global default first.
    expect(presetChatLandingPresentation).toHaveBeenCalledWith('chat-9', 'terminal')
    expect(openChat).toHaveBeenCalledWith('chat-9')
  })

  it('does not preset Terminal for a provider with no terminal at all (absence, not a disabled control)', async () => {
    fakeState.agentChats = {
      providers: [
        {
          id: 'p1',
          displayName: 'Claude',
          icon: '',
          connected: true,
          enabled: true,
          hasTerminal: false,
        },
      ],
      chats: [],
    }
    createChat.mockResolvedValue('chat-9')
    renderHook(() => usePaneKeyboard())

    pressTerminalChord()
    await createChat.mock.results[0]?.value
    await Promise.resolve()

    // Still an ordinary create — same as plain agent.newChat — just never
    // told to land somewhere this provider cannot show.
    expect(createChat).toHaveBeenCalledWith('ws-1', 'p1', '', undefined)
    expect(presetChatLandingPresentation).not.toHaveBeenCalled()
    expect(openChat).toHaveBeenCalledWith('chat-9')
  })

  // THE surfaces: fix (design spec 2.5): hasTerminal alone is not enough — a
  // provider can HAVE a terminal that is only reachable by switching to it
  // after a turn, never a landing surface for a chat that does not exist yet.
  // terminalStartHere is the fact that distinguishes them. (Both shipped
  // providers now declare it; this pins the affordance against one that does
  // not, which is what absence must keep meaning.)
  it('does not preset Terminal for a provider whose terminal is not a start_here surface, even though it has one', async () => {
    fakeState.agentChats = {
      providers: [
        {
          id: 'p1',
          displayName: 'Codex',
          icon: '',
          connected: true,
          enabled: true,
          hasTerminal: true,
          // terminalStartHere omitted: a terminal reached only by switching.
        },
      ],
      chats: [],
    }
    createChat.mockResolvedValue('chat-9')
    renderHook(() => usePaneKeyboard())

    pressTerminalChord()
    await createChat.mock.results[0]?.value
    await Promise.resolve()

    expect(presetChatLandingPresentation).not.toHaveBeenCalled()
    expect(openChat).toHaveBeenCalledWith('chat-9')
  })

  it('plain agent.newChat never presets a surface', async () => {
    fakeState.agentChats = {
      providers: [{ id: 'p1', displayName: 'Claude', icon: '', connected: true, enabled: true }],
      chats: [],
    }
    createChat.mockResolvedValue('chat-9')
    renderHook(() => usePaneKeyboard())

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'n', metaKey: true }))
    await createChat.mock.results[0]?.value
    await Promise.resolve()

    expect(presetChatLandingPresentation).not.toHaveBeenCalled()
  })
})
