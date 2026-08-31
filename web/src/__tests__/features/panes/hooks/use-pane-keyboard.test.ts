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
const setPaneChat = vi.fn()
const setActiveAgentChatId = vi.fn()

// I4: the AGENT_NEW_CHAT chord (⌘N) creates a chat via the agent API — mocked
// here the same way NewTabView's own regression tests mock it. `vi.hoisted` is
// required (not a plain const): the mock factory below reads `createChat`
// directly (an eager shorthand-property read at factory-call time, unlike the
// `() => fakeStore` closures elsewhere in this file, which defer the read).
const { createChat, toastSpawnFailure } = vi.hoisted(() => ({
  createChat: vi.fn(),
  toastSpawnFailure: vi.fn(),
}))
vi.mock('@/features/agent/api/agent-api', () => ({ createChat }))
vi.mock('@/features/agent/lib/spawn-error', () => ({ toastSpawnFailure }))

type FakePane = { activeEditorTabId: string | null; editorTabIds: string[] }
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
// where `panes` has a single entry. `rootLayout`/`bottomLayout` are the two
// independent layout trees getPaneScopeForPaneId scopes "last remaining pane"
// against (C1: a raw `Object.keys(state.panes).length` conflates the two
// trees and is never 1, even in a genuinely single-pane workspace).
const fakeState = {
  activePaneId: ROOT_PANE_ID,
  workspaceId: 'ws-1',
  rootLayout: { type: 'pane', id: ROOT_PANE_ID } as FakeLayout,
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
    }>,
    chats: [] as Array<{ id: string; title: string }>,
  },
  bufferActions: {
    closeBuffer,
    reopenLastClosedBuffer,
    setPendingClose,
    openContent,
  },
  paneActions: { navigateToPane, removeEditorTabFromPane, closePane, setActivePane, setPaneChat },
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
  }),
}))

vi.mock('@/features/panes/utils/pane-command-actions', () => ({
  splitActiveEditorGroup: vi.fn(),
}))

import { usePaneKeyboard } from '@/features/panes/hooks/use-pane-keyboard'

beforeEach(() => {
  vi.clearAllMocks()
  fakeState.activePaneId = ROOT_PANE_ID
  fakeState.agentChats = { providers: [], chats: [] }
  fakeState.panes = {
    [ROOT_PANE_ID]: { activeEditorTabId: 'buf-1', editorTabIds: ['buf-1'] },
    [BOTTOM_PANE_ID]: { activeEditorTabId: null, editorTabIds: [] },
  }
  fakeState.rootLayout = { type: 'pane', id: ROOT_PANE_ID }
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
 * splits `rootLayout` into `paneCount` root-tree leaves (padding out extra,
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
  let rootLayout: FakeLayout = { type: 'pane', id: ROOT_PANE_ID }
  for (let i = 2; i <= paneCount; i++) {
    const extraId = `pane-${i}`
    panes[extraId] = { activeEditorTabId: null, editorTabIds: [] }
    rootLayout = {
      type: 'split',
      id: `split-${i}`,
      direction: 'horizontal',
      sizes: [50, 50],
      first: rootLayout,
      second: { type: 'pane', id: extraId },
    }
  }
  fakeState.panes = panes
  fakeState.rootLayout = rootLayout
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
  fakeState.rootLayout = { type: 'pane', id: ROOT_PANE_ID }
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
    expect(setPaneChat).not.toHaveBeenCalled()
  })

  it('mod+j opens a terminal', () => {
    renderHook(() => usePaneKeyboard())
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'j', metaKey: true }))
    expect(openContent).toHaveBeenCalledWith({ type: 'terminal' })
  })

  it('mod+shift+n opens an untitled virtual buffer', () => {
    renderHook(() => usePaneKeyboard())
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'n', metaKey: true, shiftKey: true }))
    expect(openContent).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'editor', isVirtual: true }),
    )
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

  it('creates a chat with the first enabled provider and opens it on the active pane', async () => {
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
    // Provider-agnostic: picks the FIRST ENABLED provider without asking.
    expect(createChat).toHaveBeenCalledWith('ws-1', 'p1')

    fakeState.agentChats.chats = [{ id: 'chat-9', title: 'New conversation' }]
    await createChat.mock.results[0]?.value
    await Promise.resolve()

    expect(setActiveAgentChatId).toHaveBeenCalledWith('chat-9')
    // Opening a chat is no longer "adding a tab" — it sets the pane's own chat
    // slot directly, with no runner known yet.
    expect(setPaneChat).toHaveBeenCalledWith(ROOT_PANE_ID, 'chat-9', null)
    expect(openContent).not.toHaveBeenCalled()
  })

  it('does nothing when no provider is available (no CLI installed)', () => {
    fakeState.agentChats = { providers: [], chats: [] }
    renderHook(() => usePaneKeyboard())
    pressChord()
    expect(createChat).not.toHaveBeenCalled()
    expect(setPaneChat).not.toHaveBeenCalled()
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
    expect(createChat).toHaveBeenCalledWith('ws-1', 'p2')
  })

  it('does nothing when every provider is disabled', () => {
    fakeState.agentChats = {
      providers: [{ id: 'p1', displayName: 'Claude', icon: '', connected: true, enabled: false }],
      chats: [],
    }
    renderHook(() => usePaneKeyboard())
    pressChord()
    expect(createChat).not.toHaveBeenCalled()
    expect(setPaneChat).not.toHaveBeenCalled()
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
