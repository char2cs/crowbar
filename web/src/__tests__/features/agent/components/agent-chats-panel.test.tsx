/**
 * Tests for the agent-chats sidebar panel: list + the unified New-chat row +
 * drag-to-reorder + drop-to-delete.
 *
 * The workspace store is REAL (registry + agent-chats slice + buffer slice) so
 * ordering, persistence and pane-opening are exercised end to end; only the
 * network seams (agent-api, the WS stream hook) and the IDB persistence writers
 * are mocked.
 *
 * jsdom implements neither the PointerEvent constructor nor
 * document.elementsFromPoint (no layout). Drags are driven with MouseEvents
 * named "pointerdown"/"pointermove"/"pointerup" (React and the window listeners
 * duck-type by name — see pane-sash.test.tsx) and elementsFromPoint is stubbed
 * with the element the pointer is "over".
 */
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/lib/api'

// ── Mocks ───────────────────────────────────────────────────────────

const { createChatFn, deleteChatFn, renameChatFn, stopChatFn, streamHook, toastErrorFn } =
  vi.hoisted(() => ({
    createChatFn: vi.fn(),
    deleteChatFn: vi.fn(),
    renameChatFn: vi.fn(),
    // stopChat is fired by closeBuffer when a chat TAB is closed (the pane's × or a
    // close-tab flow the panel triggers). It must be exported here or the fire-and-forget
    // dynamic import in buffer-slice throws an unhandled rejection during these tests.
    stopChatFn: vi.fn(async (..._a: unknown[]) => {}),
    streamHook: vi.fn(),
    toastErrorFn: vi.fn(),
  }))

vi.mock('@/features/agent/api/agent-api', () => ({
  createChat: (...a: unknown[]) => createChatFn(...a),
  deleteChat: (...a: unknown[]) => deleteChatFn(...a),
  renameChat: (...a: unknown[]) => renameChatFn(...a),
  stopChat: (...a: unknown[]) => stopChatFn(...a),
}))

vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: (...a: unknown[]) => toastErrorFn(...a) },
}))

vi.mock('@/features/workspace/stores/hooks/use-workspace-agent-chats-stream', () => ({
  useWorkspaceAgentChatsStream: (wsId: string) => streamHook(wsId),
}))

// The list is virtualized; the real virtualizer measures DOM layout, which jsdom
// reports as zero — so it would render no rows. Replace it with a deterministic
// stand-in that renders EVERY row (the git-diff-editor-stack.test pattern), so
// every assertion about rendered rows and drag targeting still holds.
vi.mock('@tanstack/react-virtual', () => ({
  useVirtualizer: (options: { count: number; estimateSize: () => number }) => {
    const size = options.estimateSize()
    return {
      getVirtualItems: () =>
        Array.from({ length: options.count }, (_, index) => ({
          index,
          key: index,
          start: index * size,
          size,
        })),
      getTotalSize: () => options.count * size,
      scrollToIndex: () => {},
      measureElement: () => {},
    }
  },
}))

// Keep the real workspace store out of IndexedDB.
vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/editor/stores/buffer-session-persistence', () => ({
  saveSessionToStore: vi.fn(),
  clearQueuedWorkspaceSessionSave: vi.fn(),
}))

import { AgentChatsPanel } from '@/features/agent/components/agent-chats-panel'
import { reorderIds } from '@/features/agent/components/agent-chats-reorder'
import {
  destroyWorkspaceStore,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import { setActiveWorkspaceStoreRef } from '@/features/workspace/stores/workspace-store-ref'
import { parseWorkspaceScopeFromPath } from '@/lib/workspace-scope'
import type { AgentChat } from '@/features/agent/api/agent-api'

// ── Fixtures / helpers ──────────────────────────────────────────────

const ORDER_KEY = 'crowbar:agent-chat-order:w1'

const chat = (
  id: string,
  title: string,
  providerId: string,
  createdAt: string,
  workspaceId = 'w1',
): AgentChat => ({
  id,
  workspaceId,
  title,
  liveRunnerId: `${id}-r`,
  terminalSessionId: `${id}-pty`,
  activeProviderId: providerId,
  createdAt,
})

const PROVIDERS = [
  {
    id: 'claude',
    displayName: 'Claude',
    icon: '<svg data-p="claude"></svg>',
    connected: true,
    enabled: true,
    mcpEnabled: true,
  },
  {
    id: 'codex',
    displayName: 'Codex',
    icon: '<svg data-p="codex"></svg>',
    connected: true,
    enabled: true,
    mcpEnabled: true,
  },
]

const CHAT_1 = chat('c1', 'First', 'claude', '2026-01-01T00:00:00Z')
const CHAT_2 = chat('c2', 'Second', 'codex', '2026-01-02T00:00:00Z')

function state(wsId = 'w1') {
  return getOrCreateWorkspaceStore(wsId).getState()
}

/**
 * Publish `wsId` as the ACTIVE workspace — exactly what WorkspaceView does (in a
 * layout effect) on every route that mounts it, which is both the worktree route
 * (/ide/:p/:r/:w) and the project-home route (/ide/:p/home). This, not the URL,
 * is where the panel reads its wsId from.
 */
function activate(wsId: string) {
  act(() => setActiveWorkspaceStoreRef(getOrCreateWorkspaceStore(wsId)))
}

/** Seed providers + two chats (c1 older than c2). */
function seed(chats: AgentChat[] = [CHAT_1, CHAT_2], wsId = 'w1') {
  const st = state(wsId)
  act(() => {
    st.setAgentProviders(PROVIDERS)
    for (const c of chats) st.upsertAgentChat(c)
  })
}

function rowIds(): string[] {
  return Array.from(document.querySelectorAll('[data-agent-chat-drop]')).map(
    (el) => el.getAttribute('data-agent-chat-drop') ?? '',
  )
}

function rowFor(chatId: string): HTMLElement {
  const el = document.querySelector<HTMLElement>(`[data-agent-chat-drop="${chatId}"]`)
  if (!el) throw new Error(`no row for ${chatId}`)
  return el
}

function trashZone(): HTMLElement {
  const el = document.querySelector<HTMLElement>('[data-trash-drop]')
  if (!el) throw new Error('no trash zone')
  return el
}

function pointerDown(el: HTMLElement, button = 0) {
  act(() => {
    el.dispatchEvent(
      new MouseEvent('pointerdown', { button, clientX: 0, clientY: 0, bubbles: true }),
    )
  })
}

function pointerMove(x = 0, y = 40) {
  act(() => {
    window.dispatchEvent(new MouseEvent('pointermove', { clientX: x, clientY: y, bubbles: true }))
  })
}

function pointerUp(x = 0, y = 40) {
  act(() => {
    window.dispatchEvent(new MouseEvent('pointerup', { clientX: x, clientY: y, bubbles: true }))
  })
}

// The list is virtualized and the drag resolves its target from scroll geometry,
// not DOM hit-testing. jsdom has no layout, so stub the two rects the resolver
// reads: the scroll container and (for delete) the trash zone.
const ROW_H = 40

function stubRect(el: Element | null, rect: Partial<DOMRect>) {
  if (!el) throw new Error('stubRect: element not found')
  const full = { top: 0, left: 0, right: 0, bottom: 0, width: 0, height: 0, x: 0, y: 0, ...rect }
  el.getBoundingClientRect = () => ({ ...full, toJSON: () => full }) as DOMRect
}

// The scroll container starts at viewport y=0, so rowY(i) maps to row i. Tall
// enough that the rows used here clear the auto-scroll edges (auto-scroll has its
// own pure unit tests; rAF is stubbed off in beforeEach).
function stubListGeometry() {
  stubRect(document.querySelector('[data-agent-chat-scroll]'), {
    top: 0,
    left: 0,
    right: 200,
    bottom: 1000,
    width: 200,
    height: 1000,
  })
}

const TRASH_X = 10
const TRASH_Y = 2000
function stubTrashGeometry() {
  stubRect(document.querySelector('[data-trash-drop]'), {
    top: 1980,
    left: 0,
    right: 200,
    bottom: 2040,
    width: 200,
    height: 60,
  })
}

/** clientY that lands exactly on drop SLOT `slot` — the gap above row `slot`.
 *  Slots are resolved off row MIDPOINTS, so a boundary y is unambiguous. */
const slotY = (slot: number) => slot * ROW_H

/** Drag `dragId` and drop it in slot `slot` (the gap above row `slot`; `count`
 *  is the end of the list). */
function dragToSlot(dragId: string, slot: number) {
  stubListGeometry()
  pointerDown(rowFor(dragId))
  pointerMove(0, 6) // cross the 5px threshold → drag goes live
  pointerMove(0, slotY(slot))
  pointerUp(0, slotY(slot))
}

/** Drag `dragId` and release far below the list. */
function dragBelowTheList(dragId: string) {
  stubListGeometry()
  pointerDown(rowFor(dragId))
  pointerMove(0, 6)
  pointerMove(0, 900) // below every row, above the bottom edge
  pointerUp(0, 900)
}

/** The insertion line's slot, or null when no line is drawn. */
function dropLineSlot(): number | null {
  const el = document.querySelector('[data-agent-chat-drop-line]')
  return el ? Number(el.getAttribute('data-agent-chat-drop-line')) : null
}

/** Drag `dragId` onto the trash zone. */
function dragToTrash(dragId: string) {
  stubListGeometry()
  stubTrashGeometry()
  pointerDown(rowFor(dragId))
  pointerMove(0, 6)
  pointerMove(TRASH_X, TRASH_Y)
  pointerUp(TRASH_X, TRASH_Y)
}

const agentBuffers = () => state().buffers.filter((b) => b.type === 'agentChat')

beforeEach(() => {
  localStorage.clear()
  createChatFn.mockReset().mockResolvedValue('c-new')
  deleteChatFn.mockReset().mockResolvedValue(undefined)
  renameChatFn.mockReset().mockResolvedValue(undefined)
  toastErrorFn.mockReset()
  streamHook.mockClear()
  // The drag's edge auto-scroll runs in a rAF loop; jsdom can't scroll and the
  // loop's logic is covered by the pure autoScrollDelta tests, so make rAF inert
  // here (returns an id, never invokes the callback).
  vi.stubGlobal('requestAnimationFrame', () => 1)
  vi.stubGlobal('cancelAnimationFrame', () => {})
  activate('w1')
})

afterEach(async () => {
  cleanup()
  setActiveWorkspaceStoreRef(null)
  destroyWorkspaceStore('w1')
  destroyWorkspaceStore('hw1')
  // A drop arms a one-shot capture-phase click trap (the post-drop click must
  // not select the dragged row); the panel drops it on the next macrotask, as
  // the browser would. Drain that already-scheduled task so an unconsumed trap
  // never leaks into the next test.
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0))
  })
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

// ── Tests ───────────────────────────────────────────────────────────

describe('AgentChatsPanel', () => {
  it('renders nothing when no workspace is active', () => {
    act(() => setActiveWorkspaceStoreRef(null))
    const { container } = render(<AgentChatsPanel />)
    expect(container.firstChild).toBeNull()
    expect(streamHook).not.toHaveBeenCalled()
  })

  it('mounts the agent-chats WS stream for the active workspace', () => {
    seed()
    render(<AgentChatsPanel />)
    expect(streamHook).toHaveBeenCalledWith('w1')
  })

  // The project-home route is /ide/:projectId/home: no wsId in the URL (it is
  // resolved asynchronously and published by WorkspaceView). Deriving wsId from
  // the pathname left the Chats tab blank on every home workspace.
  it('renders chats + New rows on a project-home workspace (no wsId in the URL)', () => {
    expect(parseWorkspaceScopeFromPath('/ide/p1/home')).toBeNull()

    activate('hw1')
    seed([chat('h1', 'Home chat', 'claude', '2026-03-01T00:00:00Z', 'hw1')], 'hw1')
    const { container } = render(<AgentChatsPanel />)

    expect(streamHook).toHaveBeenCalledWith('hw1')
    expect(rowIds()).toEqual(['h1'])
    expect(screen.getByText('Home chat')).toBeTruthy()
    // One unified New-chat row, not one-per-provider.
    expect(container.querySelectorAll('[data-new-chat]')).toHaveLength(1)
  })

  it('follows the active workspace when it changes (worktree → project home)', () => {
    seed()
    seed([chat('h1', 'Home chat', 'claude', '2026-03-01T00:00:00Z', 'hw1')], 'hw1')
    render(<AgentChatsPanel />)
    expect(rowIds()).toEqual(['c2', 'c1'])

    activate('hw1')

    expect(rowIds()).toEqual(['h1'])
    expect(streamHook).toHaveBeenLastCalledWith('hw1')
  })

  it('renders for an explicit wsId prop (the sidebar host may pass one)', () => {
    act(() => setActiveWorkspaceStoreRef(null))
    seed()
    render(<AgentChatsPanel wsId="w1" />)
    expect(rowIds()).toEqual(['c2', 'c1'])
  })

  it('renders seeded chats NEWEST-first by createdAt', () => {
    seed()
    render(<AgentChatsPanel />)
    // c2 is the newer of the two. A chat you just started belongs at the top —
    // appending newest LAST buried it, and the New Tab's capped "Recent" list
    // (which takes its top-N from this same ordering) dropped it entirely.
    expect(rowIds()).toEqual(['c2', 'c1'])
    expect(screen.getByText('First')).toBeTruthy()
    expect(screen.getByText('Second')).toBeTruthy()
  })

  it('hydrates and applies the persisted user order', () => {
    localStorage.setItem(ORDER_KEY, JSON.stringify(['c2', 'c1']))
    seed()
    render(<AgentChatsPanel />)
    expect(rowIds()).toEqual(['c2', 'c1'])
  })

  it('resolves each row provider icon from the providers list', () => {
    seed()
    render(<AgentChatsPanel />)
    expect(rowFor('c1').querySelector('[data-p="claude"]')).not.toBeNull()
    expect(rowFor('c2').querySelector('[data-p="codex"]')).not.toBeNull()
  })

  it('falls back to the chat glyph for a chat whose provider is unknown', () => {
    seed([chat('c9', 'Orphan', 'gemini', '2026-01-03T00:00:00Z')])
    render(<AgentChatsPanel />)
    // Not the provider's icon (there isn't one) and NOT an empty slot: a chat with
    // no resolvable provider still has to read as a chat — specifically the chat glyph,
    // not merely some svg (a file icon would satisfy that and defeat the fallback's point).
    expect(rowFor('c9').querySelector('[data-provider-icon]')).toBeNull()
    expect(rowFor('c9').querySelector('[data-chat-glyph]')).not.toBeNull()
  })

  it('shows the working spinner on the working chat only', () => {
    seed()
    act(() => state().setAgentChatWorking('c2', true))
    render(<AgentChatsPanel />)
    expect(rowFor('c2').querySelector('[role="status"]')).not.toBeNull()
    expect(rowFor('c1').querySelector('[role="status"]')).toBeNull()
  })

  it('clicking a chat row selects it and opens its agentChat pane', () => {
    seed()
    render(<AgentChatsPanel />)
    fireEvent.click(screen.getByText('First'))

    expect(state().agentChats.activeChatId).toBe('c1')
    const buf = agentBuffers()
    expect(buf).toHaveLength(1)
    expect(buf[0]).toMatchObject({ type: 'agentChat', chatId: 'c1', wsId: 'w1', name: 'First' })
  })

  it('names an untitled chat tab the SAME as its row (UNTITLED_CHAT_LABEL)', () => {
    // The pane tab used to open as 'Agent chat' while the row said 'Untitled
    // chat' — the exact drift chat-label.ts's constant exists to prevent.
    seed([chat('c3', '', 'claude', '2026-01-04T00:00:00Z')])
    render(<AgentChatsPanel />)
    fireEvent.click(rowFor('c3'))
    expect(agentBuffers()[0]).toMatchObject({ chatId: 'c3', name: 'Untitled chat' })
  })

  it('marks the active chat row active', () => {
    seed()
    render(<AgentChatsPanel />)
    fireEvent.click(rowFor('c2'))
    expect(rowFor('c2').className).toContain('bg-background')
    expect(rowFor('c1').className).toContain('hover:bg-accent')
  })

  // ── A row is lit by its TAB, not by the last click ────────────────

  it('lights every chat that has a tab open, and darkens one whose tab is closed', () => {
    seed()
    render(<AgentChatsPanel />)

    fireEvent.click(rowFor('c1'))
    fireEvent.click(rowFor('c2'))
    // Two tabs open ⇒ two lit rows. The row reflects the tab strip, not a single
    // "selected" chat.
    expect(rowFor('c1').className).toContain('bg-background')
    expect(rowFor('c2').className).toContain('bg-background')

    const c1Buffer = agentBuffers().find((b) => b.chatId === 'c1')!
    act(() => state().bufferActions.closeBuffer(c1Buffer.id))

    // Closing the tab must put the row out — the old highlight followed a stored
    // activeChatId that nothing cleared, so a closed chat stayed lit forever.
    expect(rowFor('c1').className).toContain('hover:bg-accent')
    expect(rowFor('c2').className).toContain('bg-background')
  })

  it('clicking a chat already open in ANOTHER pane reveals that pane, never moving the tab', () => {
    seed()
    render(<AgentChatsPanel />)

    fireEvent.click(rowFor('c1'))
    const homePane = state().activePaneId
    const bufferId = agentBuffers()[0].id

    // Split, and stand in the NEW pane — the chat's tab is now off in the other half.
    let otherPane = ''
    act(() => {
      otherPane = state().paneActions.splitPane(homePane, 'horizontal') ?? ''
    })
    expect(state().activePaneId).toBe(otherPane)

    fireEvent.click(rowFor('c1'))

    // Go to the tab; don't drag the tab to us. openContent would have added the
    // buffer to the active pane, tearing the chat out of the pane it lives in.
    expect(state().activePaneId).toBe(homePane)
    expect(state().panes[homePane].activeBufferId).toBe(bufferId)
    expect(state().panes[otherPane].bufferIds).not.toContain(bufferId)
    expect(agentBuffers()).toHaveLength(1)
  })

  // ── Unified New-chat row ──────────────────────────────────────────

  it('renders exactly one New chat row BELOW every chat row, + on the right', () => {
    seed()
    const { container } = render(<AgentChatsPanel />)

    // One unified row — not one-per-provider. It opens the FIRST ENABLED provider
    // (claude here), so the row identifies that provider, and sits after the chats.
    const rows = Array.from(container.querySelectorAll('[data-agent-chat-drop], [data-new-chat]'))
    expect(
      rows.map((r) => r.getAttribute('data-agent-chat-drop') ?? r.getAttribute('data-new-chat')),
    ).toEqual(['c2', 'c1', 'claude'])

    // A single, provider-agnostic label — no more "New Claude chat"/"New Codex chat".
    expect(screen.getAllByText('New chat')).toHaveLength(1)
    expect(screen.queryByText('New Claude chat')).toBeNull()
    expect(screen.queryByText('New Codex chat')).toBeNull()

    // Separator sits above the row while there are chats to divide from.
    expect(container.querySelector('[data-new-chat-separator]')).not.toBeNull()

    // The + glyph is the row's LAST child (right edge); the provider icon leads.
    const newRow = container.querySelector<HTMLElement>('[data-new-chat="claude"]')!
    expect(newRow.firstElementChild?.querySelector('[data-p="claude"]')).not.toBeNull()
    expect(newRow.lastElementChild?.getAttribute('data-add-glyph')).toBe('true')
  })

  it('opens the FIRST ENABLED provider — a disabled leading provider is skipped', async () => {
    // claude is disabled, codex enabled → the row must open codex, never claude.
    // (seed() resets providers, so override the set AFTER it.)
    seed([CHAT_1])
    act(() =>
      state().setAgentProviders([
        {
          id: 'claude',
          displayName: 'Claude',
          icon: '<svg data-p="claude"></svg>',
          connected: true,
          enabled: false,
          mcpEnabled: true,
        },
        {
          id: 'codex',
          displayName: 'Codex',
          icon: '<svg data-p="codex"></svg>',
          connected: true,
          enabled: true,
          mcpEnabled: true,
        },
      ]),
    )
    const { container } = render(<AgentChatsPanel />)

    const newRow = container.querySelector<HTMLElement>('[data-new-chat]')!
    expect(newRow.getAttribute('data-new-chat')).toBe('codex')
    fireEvent.click(newRow)
    expect(createChatFn).toHaveBeenCalledWith('w1', 'codex')
    await waitFor(() => expect(agentBuffers()).toHaveLength(1))
  })

  /** Disable every provider. seed() resets them, so this runs AFTER it. */
  function disableAllProviders() {
    act(() =>
      state().setAgentProviders([
        {
          id: 'claude',
          displayName: 'Claude',
          icon: '',
          connected: true,
          enabled: false,
          mcpEnabled: true,
        },
        {
          id: 'codex',
          displayName: 'Codex',
          icon: '',
          connected: true,
          enabled: false,
          mcpEnabled: true,
        },
      ]),
    )
  }

  it('renders no New chat row when no provider is enabled', () => {
    seed([CHAT_1])
    disableAllProviders()
    const { container } = render(<AgentChatsPanel />)
    expect(container.querySelectorAll('[data-new-chat]')).toHaveLength(0)
    expect(container.querySelector('[data-new-chat-separator]')).toBeNull()
  })

  // …and says WHY, instead of leaving a dead surface. With every provider off
  // and no chats yet, this panel rendered COMPLETELY BLANK: no row, no message,
  // nothing to click and nothing to read.
  it('explains itself when no provider is enabled, and names where to fix it', () => {
    seed([])
    disableAllProviders()
    render(<AgentChatsPanel />)

    expect(screen.getByTestId('no-providers-notice')).toBeInTheDocument()
    expect(screen.getByText(/settings → providers/i)).toBeInTheDocument()
  })

  it('still explains itself when the workspace HAS chats but nothing can start one', () => {
    seed([CHAT_1])
    disableAllProviders()
    render(<AgentChatsPanel />)

    expect(screen.getByTestId('no-providers-notice')).toBeInTheDocument()
    expect(screen.getByText('First')).toBeInTheDocument() // the existing chats still list
  })

  it('shows no notice while a provider is enabled', () => {
    seed()
    render(<AgentChatsPanel />)
    expect(screen.queryByTestId('no-providers-notice')).toBeNull()
  })

  // Regression: NewChatRow is a real <button>, whose UA text-align is `center`.
  // Its label must carry text-left or it centers in the flex-1 span (the "buttons
  // got broken" report after the div→button role-vs-tag swap). Every other
  // button-based sidebar row (project-switcher's "Import project", the tree's
  // "New") already compensates the same way.
  it('the New-row label is left-aligned, not centered by the <button> default', () => {
    seed()
    render(<AgentChatsPanel />)
    const label = screen.getByText('New chat')
    expect(label.className).toContain('text-left')
  })

  // Regression: `w-full` on the button = width:100% of the parent, but ROW_BASE
  // also has mx-1.5 (6px each side), so the row pokes 6px past the sidebar →
  // a stray horizontal scrollbar in the Chats panel (jsdom can't measure the
  // overflow, so guard the two structural facts that prevent it). The rows fill
  // via the list container being flex-col (each row stretches to width − margin);
  // a <button> is shrink-to-fit at display:flex, so it must NOT rely on w-full.
  it('the New rows fill width via flex-col stretch, never w-full (which overflows the sidebar)', () => {
    seed()
    const { container } = render(<AgentChatsPanel />)
    const btn = container.querySelector<HTMLElement>('[data-new-chat="claude"]')!
    expect(btn.className).not.toContain('w-full')
    expect(btn.parentElement?.className).toContain('flex-col')
  })

  it('empty state renders only the single New chat row', () => {
    act(() => state().setAgentProviders(PROVIDERS))
    const { container } = render(<AgentChatsPanel />)
    expect(rowIds()).toEqual([])
    expect(container.querySelectorAll('[data-new-chat]')).toHaveLength(1)
    // With no chats to divide from, no separator is drawn above the row.
    expect(container.querySelector('[data-new-chat-separator]')).toBeNull()
  })

  it('renders no separator and no New rows when providers have not arrived', () => {
    seed([CHAT_1])
    act(() => state().setAgentProviders([]))
    const { container } = render(<AgentChatsPanel />)
    expect(container.querySelectorAll('[data-new-chat]')).toHaveLength(0)
    expect(container.querySelector('[data-new-chat-separator]')).toBeNull()
  })

  it('clicking the New chat row creates a chat for the first enabled provider and opens its pane', async () => {
    seed()
    render(<AgentChatsPanel />)
    fireEvent.click(screen.getByText('New chat'))

    // claude is the first enabled provider in priority order.
    expect(createChatFn).toHaveBeenCalledWith('w1', 'claude')
    await waitFor(() => expect(agentBuffers()).toHaveLength(1))
    expect(agentBuffers()[0]).toMatchObject({ chatId: 'c-new', wsId: 'w1', name: 'Claude chat' })
    expect(state().agentChats.activeChatId).toBe('c-new')
  })

  it('names the new tab from the chat title when the WS seed already landed it', async () => {
    seed()
    createChatFn.mockImplementation(() => {
      // The 'created' WS frame reseeds the list before the POST promise resolves.
      state().upsertAgentChat(chat('c-new', 'Seeded title', 'claude', '2026-02-01T00:00:00Z'))
      return Promise.resolve('c-new')
    })
    render(<AgentChatsPanel />)
    fireEvent.click(screen.getByText('New chat'))

    await waitFor(() => expect(agentBuffers()).toHaveLength(1))
    expect(agentBuffers()[0]).toMatchObject({ chatId: 'c-new', name: 'Seeded title' })
  })

  it('a New row is a real <button> — keyboard-operable via Enter/Space, inert on other keys', async () => {
    seed()
    const user = userEvent.setup()
    const { container } = render(<AgentChatsPanel />)
    const newRow = container.querySelector<HTMLElement>('[data-new-chat="claude"]')!

    // NewChatRow renders a real <button> (see role-vs-tag fix), so Enter/Space
    // activation is the browser's own default action, not app code — exercised
    // here via user-event (which, unlike bare fireEvent.keyDown, simulates that
    // default action for native form elements) rather than reimplemented by hand.
    expect(newRow.tagName).toBe('BUTTON')
    newRow.focus()

    await user.keyboard('a')
    expect(createChatFn).not.toHaveBeenCalled()

    await user.keyboard('{Enter}')
    await user.keyboard(' ')
    expect(createChatFn).toHaveBeenCalledTimes(2)
    expect(createChatFn).toHaveBeenLastCalledWith('w1', 'claude')

    // Both creates settle inside act() — the pane opens once (openContent dedupes
    // on chatId), and no state lands after the test ends.
    await waitFor(() => expect(agentBuffers()).toHaveLength(1))
  })

  it('a failed create opens no pane', async () => {
    const err = vi.spyOn(console, 'error').mockImplementation(() => {})
    createChatFn.mockRejectedValue(new Error('boom'))
    seed()
    render(<AgentChatsPanel />)
    fireEvent.click(screen.getByText('New chat'))

    await waitFor(() => expect(err).toHaveBeenCalled())
    expect(agentBuffers()).toHaveLength(0)
    expect(state().agentChats.activeChatId).toBeNull()
  })

  // The bug the user actually hit: the daemon could not find the claude binary (launchd's
  // PATH omits ~/.local/bin), every create 500'd, and this catch dropped it into
  // console.error — so the chat button did NOTHING, over and over, with no way to learn
  // why. A failed create must SAY something.
  it('a failed create tells the user why, naming the CLI that is missing', async () => {
    const err = vi.spyOn(console, 'error').mockImplementation(() => {})
    createChatFn.mockRejectedValue(new ApiError('terminal: command not found: claude', 424))
    seed()
    render(<AgentChatsPanel />)
    fireEvent.click(screen.getByText('New chat'))

    await waitFor(() => expect(toastErrorFn).toHaveBeenCalledTimes(1))
    const [title, description] = toastErrorFn.mock.calls[0] as [string, string]
    expect(title).toContain('Claude')
    expect(title).toMatch(/isn.t installed/)
    expect(description).toMatch(/PATH/)
    err.mockRestore()
  })

  // ── Rename ────────────────────────────────────────────────────────

  it('double-click renames: optimistic title + renameChat call', () => {
    seed()
    render(<AgentChatsPanel />)
    fireEvent.doubleClick(screen.getByText('First'))

    const input = screen.getByDisplayValue('First')
    fireEvent.change(input, { target: { value: 'Renamed' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    expect(renameChatFn).toHaveBeenCalledWith('w1', 'c1', 'Renamed')
    expect(state().agentChats.chats.find((c) => c.id === 'c1')?.title).toBe('Renamed')
    expect(screen.queryByDisplayValue('Renamed')).toBeNull()
  })

  it('confirming the same title neither calls the API nor touches the store', () => {
    seed()
    render(<AgentChatsPanel />)
    fireEvent.doubleClick(screen.getByText('First'))
    fireEvent.keyDown(screen.getByDisplayValue('First'), { key: 'Enter' })

    expect(renameChatFn).not.toHaveBeenCalled()
    expect(screen.getByText('First')).toBeTruthy()
  })

  it('a failed rename is logged (the WS title_set frame remains the source of truth)', async () => {
    const err = vi.spyOn(console, 'error').mockImplementation(() => {})
    renameChatFn.mockRejectedValue(new Error('nope'))
    seed()
    render(<AgentChatsPanel />)

    fireEvent.doubleClick(screen.getByText('First'))
    const input = screen.getByDisplayValue('First')
    fireEvent.change(input, { target: { value: 'Renamed' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    await waitFor(() => expect(err).toHaveBeenCalled())
  })

  it('cancelling a rename leaves the title untouched', () => {
    seed()
    render(<AgentChatsPanel />)
    fireEvent.doubleClick(screen.getByText('First'))
    fireEvent.keyDown(screen.getByDisplayValue('First'), { key: 'Escape' })

    expect(renameChatFn).not.toHaveBeenCalled()
    expect(screen.getByText('First')).toBeTruthy()
  })

  // ── Drag: reorder ─────────────────────────────────────────────────

  // Newest-first, so three chats render [c3, c2, c1] and every slot 0..3 is a
  // distinct destination — which is the whole point: the old row-based drop
  // could reach neither "one row down" nor "the end".
  const THREE = [CHAT_1, CHAT_2, chat('c3', 'Third', 'claude', '2026-01-03T00:00:00Z')]

  it('dropping on the LOWER half of the row directly beneath moves past it', () => {
    // The reported no-op: the drop resolved a ROW and inserted BEFORE it, so
    // dragging a row onto its own neighbour asked for the list it already was —
    // after lighting that row up as if something would happen. Halves are what
    // make "below this row" expressible.
    seed(THREE)
    render(<AgentChatsPanel />)
    expect(rowIds()).toEqual(['c3', 'c2', 'c1'])

    stubListGeometry()
    pointerDown(rowFor('c3'))
    pointerMove(0, 6)
    pointerMove(0, ROW_H + 30) // row 1 (c2), below its midpoint
    pointerUp(0, ROW_H + 30)

    expect(rowIds()).toEqual(['c2', 'c3', 'c1'])
    expect(state().agentChats.order).toEqual(['c2', 'c3', 'c1'])
  })

  it('dragging a row down one slot moves it past its neighbour', () => {
    seed(THREE)
    render(<AgentChatsPanel />)

    dragToSlot('c3', 2) // the gap between c2 and c1

    expect(rowIds()).toEqual(['c2', 'c3', 'c1'])
    expect(state().agentChats.order).toEqual(['c2', 'c3', 'c1'])
  })

  it('a drag can reach the LAST slot', () => {
    seed(THREE)
    render(<AgentChatsPanel />)

    dragToSlot('c3', 3) // past the last row — no row lives here

    expect(rowIds()).toEqual(['c2', 'c1', 'c3'])
    expect(state().agentChats.order).toEqual(['c2', 'c1', 'c3'])
  })

  it('a drag can reach the FIRST slot', () => {
    seed(THREE)
    render(<AgentChatsPanel />)

    dragToSlot('c1', 0)

    expect(rowIds()).toEqual(['c1', 'c3', 'c2'])
    expect(state().agentChats.order).toEqual(['c1', 'c3', 'c2'])
  })

  it('persists the new order', () => {
    seed()
    render(<AgentChatsPanel />)

    dragToSlot('c2', 2) // [c2, c1] → the end

    expect(rowIds()).toEqual(['c1', 'c2'])
    expect(JSON.parse(localStorage.getItem(ORDER_KEY) ?? '[]')).toEqual(['c1', 'c2'])
  })

  it('dims the dragged row and draws the insertion line in the slot the drop would land in', () => {
    seed(THREE)
    render(<AgentChatsPanel />)
    stubListGeometry()

    expect(dropLineSlot()).toBeNull() // nothing is being dragged

    pointerDown(rowFor('c3'))
    pointerMove(0, slotY(2))
    expect(rowFor('c3').className).toContain('opacity-40')
    // A LINE BETWEEN ROWS, not a ring on one: a ring can only ever say "in front
    // of this row", which is why the last slot had no affordance at all — and
    // why the ring on the row directly below the dragged one promised a move
    // that produced the identical list.
    expect(dropLineSlot()).toBe(2)

    pointerMove(0, slotY(3))
    expect(dropLineSlot()).toBe(3)

    pointerUp(0, slotY(3))
    expect(dropLineSlot()).toBeNull()
    expect(rowFor('c3').className).not.toContain('opacity-40')
  })

  it('a drag carries a ghost labelled with the chat, which follows the cursor and leaves on drop', () => {
    seed()
    render(<AgentChatsPanel />)
    const ghost = () => document.querySelector<HTMLElement>('[data-drag-ghost]')

    expect(ghost()).toBeNull()

    stubListGeometry()
    pointerDown(rowFor('c2'))
    pointerMove(30, 40) // crosses the 5px threshold → drag starts

    // Without this the drag was invisible: nothing under the cursor said WHAT was
    // being dragged, only the dimmed row left behind.
    expect(ghost()).not.toBeNull()
    expect(ghost()?.textContent).toBe('Second')

    pointerMove(80, 90)
    expect(ghost()?.style.left).toBe('92px') // 80 + DRAG_GHOST_OFFSET_X
    expect(ghost()?.style.top).toBe('80px') // 90 + DRAG_GHOST_OFFSET_Y

    pointerUp()
    expect(ghost()).toBeNull()
  })

  it('a drag that never crosses the threshold does not reorder (and the click still selects)', () => {
    seed()
    render(<AgentChatsPanel />)

    pointerDown(rowFor('c2'))
    pointerMove(2, 2) // below the 5px threshold
    pointerUp(2, 2)
    fireEvent.click(rowFor('c2'))

    expect(state().agentChats.order).toEqual([])
    expect(state().agentChats.activeChatId).toBe('c2')
  })

  it('the click that ends a real drag never selects the dragged row', () => {
    seed()
    render(<AgentChatsPanel />)

    dragToSlot('c2', 2)
    fireEvent.click(rowFor('c2')) // the browser-generated post-drop click

    expect(state().agentChats.activeChatId).toBeNull()
    expect(agentBuffers()).toHaveLength(0)
  })

  // There is no "over nothing" inside the list any more: a slot is always
  // resolved, the line always says which, and the drop always does what the line
  // showed. Below the last row is the LAST slot — the destination that used to
  // be unreachable from anywhere.
  it('dropping below the last row moves the chat to the end, as the line showed', () => {
    seed()
    render(<AgentChatsPanel />)

    dragBelowTheList('c2')

    expect(rowIds()).toEqual(['c1', 'c2'])
    expect(state().agentChats.order).toEqual(['c1', 'c2'])
    expect(deleteChatFn).not.toHaveBeenCalled()
  })

  it('a non-primary button never starts a drag', () => {
    seed()
    render(<AgentChatsPanel />)

    pointerDown(rowFor('c2'), 2)
    pointerMove()
    pointerUp()

    expect(state().agentChats.order).toEqual([])
    expect(trashZone().parentElement?.parentElement?.className).toContain('max-h-0')
  })

  it('pointercancel aborts an in-flight drag', () => {
    seed()
    render(<AgentChatsPanel />)
    stubListGeometry()

    pointerDown(rowFor('c2'))
    pointerMove()
    act(() => window.dispatchEvent(new MouseEvent('pointercancel', { bubbles: true })))

    expect(trashZone().parentElement?.parentElement?.className).toContain('max-h-0')

    pointerUp()
    expect(state().agentChats.order).toEqual([])
  })

  // ── Drag: drop-to-delete ──────────────────────────────────────────

  it('the trash zone only slides in while dragging', () => {
    seed()
    render(<AgentChatsPanel />)
    stubListGeometry()
    const slider = () => trashZone().parentElement!.parentElement!

    expect(slider().className).toContain('max-h-0')

    pointerDown(rowFor('c1'))
    pointerMove()
    expect(slider().className).toContain('max-h-16')

    pointerUp()
    expect(slider().className).toContain('max-h-0')
  })

  it('highlights the trash zone while hovering it mid-drag', () => {
    seed()
    render(<AgentChatsPanel />)
    stubListGeometry()
    stubTrashGeometry()

    pointerDown(rowFor('c1'))
    pointerMove(0, 900) // live, over nothing
    expect(trashZone().className).toContain('border-destructive/40')

    pointerMove(TRASH_X, TRASH_Y) // over the trash zone
    expect(trashZone().className).toContain('bg-destructive/10')
  })

  it('dropping a row on the trash deletes it with no confirm and closes its pane tab', async () => {
    seed()
    render(<AgentChatsPanel />)
    fireEvent.click(rowFor('c1')) // open its pane first
    expect(agentBuffers()).toHaveLength(1)

    dragToTrash('c1')

    expect(deleteChatFn).toHaveBeenCalledWith('w1', 'c1')
    expect(screen.queryByRole('alertdialog')).toBeNull()
    expect(rowIds()).toEqual(['c2'])
    expect(agentBuffers()).toHaveLength(0)
    expect(state().agentChats.activeChatId).toBeNull()
    await waitFor(() => expect(deleteChatFn).toHaveBeenCalledTimes(1))
  })

  it('deleting the ACTIVE tab of a multi-tab pane activates the sibling, not a blank pane', async () => {
    // The blank-pane bug: with two chat tabs in one pane and the FIRST active, deleting it
    // via raw closeBuffer filtered the buffer out but left the pane's activeBufferId
    // pointing at the now-gone buffer, so the pane rendered empty until the user clicked the
    // surviving tab. A single-tab delete (the test above) can't catch this — emptying a pane
    // is the correct outcome there. removeBufferFromPane is what activates the sibling.
    seed()
    render(<AgentChatsPanel />)
    fireEvent.click(rowFor('c1')) // tab 1
    fireEvent.click(rowFor('c2')) // tab 2, same pane
    fireEvent.click(rowFor('c1')) // make c1 the active tab
    const pane = state().activePaneId
    const survivor = agentBuffers().find((b) => b.chatId === 'c2')!.id
    expect(state().panes[pane].bufferIds).toHaveLength(2)

    dragToTrash('c1')

    // The deleted tab is gone AND the pane fell through to its sibling — not left pointing
    // at a buffer that no longer exists (which renders the pane's empty state).
    expect(state().panes[pane].bufferIds).toEqual([survivor])
    expect(state().panes[pane].activeBufferId).toBe(survivor)
    await waitFor(() => expect(deleteChatFn).toHaveBeenCalledWith('w1', 'c1'))
  })

  it('deleting a chat with no open pane tab is a no-op on the buffers', () => {
    seed()
    render(<AgentChatsPanel />)

    dragToTrash('c1')

    expect(deleteChatFn).toHaveBeenCalledWith('w1', 'c1')
    expect(agentBuffers()).toHaveLength(0)
  })

  it('a failed delete snaps the chat back into the list and reopens its pane tab', async () => {
    const err = vi.spyOn(console, 'error').mockImplementation(() => {})
    deleteChatFn.mockRejectedValue(new Error('nope'))
    seed()
    localStorage.setItem(ORDER_KEY, JSON.stringify(['c1', 'c2']))
    render(<AgentChatsPanel />)
    fireEvent.click(rowFor('c1')) // open its pane tab first

    dragToTrash('c1')
    expect(rowIds()).toEqual(['c2'])
    expect(agentBuffers()).toHaveLength(0)

    await waitFor(() => expect(rowIds()).toEqual(['c1', 'c2']))
    expect(state().agentChats.order).toEqual(['c1', 'c2'])
    // The optimistically closed tab comes back with the chat — and stays active.
    expect(agentBuffers()).toHaveLength(1)
    expect(agentBuffers()[0]).toMatchObject({ type: 'agentChat', chatId: 'c1', name: 'First' })
    expect(state().agentChats.activeChatId).toBe('c1')
    expect(err).toHaveBeenCalled()
  })
})

// Slot semantics: slot i is the GAP ABOVE row i, so slot `length` is the end of
// the list. Insertion is expressed against the list as the user sees it (the
// dragged row still in place), which is also what the insertion line draws.
describe('reorderIds', () => {
  it('moves the dragged id to the head', () => {
    expect(reorderIds(['a', 'b', 'c'], 'c', 0)).toEqual(['c', 'a', 'b'])
  })

  it('moves a row DOWN by one — the case "insert before the target row" could not express', () => {
    // The gap below b is slot 2. Under the old rule this dropped a "before b",
    // which is where a already was: a visible drop affordance, zero movement.
    expect(reorderIds(['a', 'b', 'c'], 'a', 2)).toEqual(['b', 'a', 'c'])
  })

  it('moves the dragged id to the LAST slot', () => {
    // slot === length: the end of the list, previously unreachable.
    expect(reorderIds(['a', 'b', 'c'], 'a', 3)).toEqual(['b', 'c', 'a'])
  })

  it('is a no-op for the two slots that touch the dragged row', () => {
    expect(reorderIds(['a', 'b', 'c'], 'b', 1)).toEqual(['a', 'b', 'c'])
    expect(reorderIds(['a', 'b', 'c'], 'b', 2)).toEqual(['a', 'b', 'c'])
  })

  it('clamps an out-of-range slot and ignores an unknown id', () => {
    expect(reorderIds(['a', 'b', 'c'], 'a', 99)).toEqual(['b', 'c', 'a'])
    expect(reorderIds(['a', 'b', 'c'], 'zz', 0)).toEqual(['a', 'b', 'c'])
  })
})
