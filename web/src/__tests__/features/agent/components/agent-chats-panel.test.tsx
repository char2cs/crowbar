/**
 * Tests for the agent-chats sidebar panel: list + New-per-provider rows +
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
import React from 'react'
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// ── Mocks ───────────────────────────────────────────────────────────

const { createChatFn, deleteChatFn, renameChatFn, streamHook } = vi.hoisted(() => ({
  createChatFn: vi.fn(),
  deleteChatFn: vi.fn(),
  renameChatFn: vi.fn(),
  streamHook: vi.fn(),
}))

vi.mock('@/features/agent/api/agent-api', () => ({
  createChat: (...a: unknown[]) => createChatFn(...a),
  deleteChat: (...a: unknown[]) => deleteChatFn(...a),
  renameChat: (...a: unknown[]) => renameChatFn(...a),
}))

vi.mock('@/features/workspace/stores/hooks/use-workspace-agent-chats-stream', () => ({
  useWorkspaceAgentChatsStream: (wsId: string) => streamHook(wsId),
}))

// ScrollArea's internal ResizeObserver state causes act() warnings — passthrough.
vi.mock('@/components/ui/scroll-area', () => ({
  ScrollArea: ({ children, className }: { children: React.ReactNode; className?: string }) => (
    <div className={className}>{children}</div>
  ),
}))

// Keep the real workspace store out of IndexedDB.
vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/editor/stores/buffer-session-persistence', () => ({
  saveSessionToStore: vi.fn(),
  clearQueuedWorkspaceSessionSave: vi.fn(),
}))

import { AgentChatsPanel, reorderIds } from '@/features/agent/components/agent-chats-panel'
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
  { id: 'claude', displayName: 'Claude', icon: '<svg data-p="claude"></svg>' },
  { id: 'codex', displayName: 'Codex', icon: '<svg data-p="codex"></svg>' },
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

/** Point the (stubbed) hit test at `el` — null means "over nothing". */
function pointerOver(el: Element | null) {
  document.elementsFromPoint = () => (el ? [el] : [])
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

/** Full drag of `chatId` onto `target` (a row element or the trash zone). */
function dragOnto(chatId: string, target: Element | null) {
  pointerOver(null)
  pointerDown(rowFor(chatId))
  pointerMove() // crosses the 5px threshold → drag starts
  pointerOver(target)
  pointerMove()
  pointerUp()
}

const agentBuffers = () => state().buffers.filter((b) => b.type === 'agentChat')

beforeEach(() => {
  localStorage.clear()
  createChatFn.mockReset().mockResolvedValue('c-new')
  deleteChatFn.mockReset().mockResolvedValue(undefined)
  renameChatFn.mockReset().mockResolvedValue(undefined)
  streamHook.mockClear()
  document.elementsFromPoint = () => []
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
    expect(container.querySelectorAll('[data-new-chat]')).toHaveLength(2)
  })

  it('follows the active workspace when it changes (worktree → project home)', () => {
    seed()
    seed([chat('h1', 'Home chat', 'claude', '2026-03-01T00:00:00Z', 'hw1')], 'hw1')
    render(<AgentChatsPanel />)
    expect(rowIds()).toEqual(['c1', 'c2'])

    activate('hw1')

    expect(rowIds()).toEqual(['h1'])
    expect(streamHook).toHaveBeenLastCalledWith('hw1')
  })

  it('renders for an explicit wsId prop (the sidebar host may pass one)', () => {
    act(() => setActiveWorkspaceStoreRef(null))
    seed()
    render(<AgentChatsPanel wsId="w1" />)
    expect(rowIds()).toEqual(['c1', 'c2'])
  })

  it('renders seeded chats oldest-first (newest last) by createdAt', () => {
    seed()
    render(<AgentChatsPanel />)
    expect(rowIds()).toEqual(['c1', 'c2'])
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

  it('falls back to a placeholder tab name for an untitled chat', () => {
    seed([chat('c3', '', 'claude', '2026-01-04T00:00:00Z')])
    render(<AgentChatsPanel />)
    fireEvent.click(rowFor('c3'))
    expect(agentBuffers()[0]).toMatchObject({ chatId: 'c3', name: 'Agent chat' })
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

  // ── New-per-provider rows ─────────────────────────────────────────

  it('renders one New row per provider BELOW every chat row, + on the right', () => {
    seed()
    const { container } = render(<AgentChatsPanel />)

    const rows = Array.from(container.querySelectorAll('[data-agent-chat-drop], [data-new-chat]'))
    expect(
      rows.map((r) => r.getAttribute('data-agent-chat-drop') ?? r.getAttribute('data-new-chat')),
    ).toEqual(['c1', 'c2', 'claude', 'codex'])

    expect(screen.getByText('New Claude chat')).toBeTruthy()
    expect(screen.getByText('New Codex chat')).toBeTruthy()

    // The + glyph is the row's LAST child (right edge); the provider icon leads.
    const newRow = container.querySelector<HTMLElement>('[data-new-chat="claude"]')!
    expect(newRow.firstElementChild?.querySelector('[data-p="claude"]')).not.toBeNull()
    expect(newRow.lastElementChild?.getAttribute('data-add-glyph')).toBe('true')
  })

  it('empty state renders only the New rows', () => {
    act(() => state().setAgentProviders(PROVIDERS))
    const { container } = render(<AgentChatsPanel />)
    expect(rowIds()).toEqual([])
    expect(container.querySelectorAll('[data-new-chat]')).toHaveLength(2)
  })

  it('renders no separator and no New rows when providers have not arrived', () => {
    seed([CHAT_1])
    act(() => state().setAgentProviders([]))
    const { container } = render(<AgentChatsPanel />)
    expect(container.querySelectorAll('[data-new-chat]')).toHaveLength(0)
    expect(container.querySelector('[data-new-chat-separator]')).toBeNull()
  })

  it('clicking a New row creates a chat for that provider and opens its pane', async () => {
    seed()
    render(<AgentChatsPanel />)
    fireEvent.click(screen.getByText('New Codex chat'))

    expect(createChatFn).toHaveBeenCalledWith('w1', 'codex')
    await waitFor(() => expect(agentBuffers()).toHaveLength(1))
    expect(agentBuffers()[0]).toMatchObject({ chatId: 'c-new', wsId: 'w1', name: 'Codex chat' })
    expect(state().agentChats.activeChatId).toBe('c-new')
  })

  it('names the new tab from the chat title when the WS seed already landed it', async () => {
    seed()
    createChatFn.mockImplementation(() => {
      // The 'created' WS frame reseeds the list before the POST promise resolves.
      state().upsertAgentChat(chat('c-new', 'Seeded title', 'codex', '2026-02-01T00:00:00Z'))
      return Promise.resolve('c-new')
    })
    render(<AgentChatsPanel />)
    fireEvent.click(screen.getByText('New Codex chat'))

    await waitFor(() => expect(agentBuffers()).toHaveLength(1))
    expect(agentBuffers()[0]).toMatchObject({ chatId: 'c-new', name: 'Seeded title' })
  })

  it('a New row is keyboard-operable (Enter / Space), and ignores other keys', async () => {
    seed()
    const { container } = render(<AgentChatsPanel />)
    const newRow = container.querySelector<HTMLElement>('[data-new-chat="claude"]')!

    fireEvent.keyDown(newRow, { key: 'Tab' })
    expect(createChatFn).not.toHaveBeenCalled()

    fireEvent.keyDown(newRow, { key: 'Enter' })
    fireEvent.keyDown(newRow, { key: ' ' })
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
    fireEvent.click(screen.getByText('New Claude chat'))

    await waitFor(() => expect(err).toHaveBeenCalled())
    expect(agentBuffers()).toHaveLength(0)
    expect(state().agentChats.activeChatId).toBeNull()
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

  it('dragging a row onto another reorders the list and persists the order', () => {
    seed()
    render(<AgentChatsPanel />)

    dragOnto('c2', rowFor('c1'))

    expect(rowIds()).toEqual(['c2', 'c1'])
    expect(state().agentChats.order).toEqual(['c2', 'c1'])
    expect(JSON.parse(localStorage.getItem(ORDER_KEY) ?? '[]')).toEqual(['c2', 'c1'])
  })

  it('dims the dragged row and rings the row the drop would land in front of', () => {
    seed()
    render(<AgentChatsPanel />)

    pointerOver(null)
    pointerDown(rowFor('c2'))
    pointerMove() // crosses the threshold → the drag is live
    expect(rowFor('c2').className).toContain('opacity-40')
    expect(rowFor('c1').className).not.toContain('ring-1 ring-ring')

    pointerOver(rowFor('c1'))
    pointerMove()
    expect(rowFor('c1').className).toContain('ring-1 ring-ring')

    pointerUp()
    expect(rowFor('c1').className).not.toContain('ring-1 ring-ring')
    expect(rowFor('c2').className).not.toContain('opacity-40')
  })

  it('a drag carries a ghost labelled with the chat, which follows the cursor and leaves on drop', () => {
    seed()
    render(<AgentChatsPanel />)
    const ghost = () => document.querySelector<HTMLElement>('[data-drag-ghost]')

    expect(ghost()).toBeNull()

    pointerOver(null)
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

    pointerOver(rowFor('c1'))
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

    dragOnto('c2', rowFor('c1'))
    fireEvent.click(rowFor('c2')) // the browser-generated post-drop click

    expect(state().agentChats.activeChatId).toBeNull()
    expect(agentBuffers()).toHaveLength(0)
  })

  it('dropping over nothing leaves the order untouched', () => {
    seed()
    render(<AgentChatsPanel />)

    dragOnto('c2', null)

    expect(rowIds()).toEqual(['c1', 'c2'])
    expect(state().agentChats.order).toEqual([])
    expect(deleteChatFn).not.toHaveBeenCalled()
  })

  it('a non-primary button never starts a drag', () => {
    seed()
    render(<AgentChatsPanel />)

    pointerDown(rowFor('c2'), 2)
    pointerOver(rowFor('c1'))
    pointerMove()
    pointerUp()

    expect(state().agentChats.order).toEqual([])
    expect(trashZone().parentElement?.parentElement?.className).toContain('max-h-0')
  })

  it('pointercancel aborts an in-flight drag', () => {
    seed()
    render(<AgentChatsPanel />)

    pointerDown(rowFor('c2'))
    pointerMove()
    act(() => window.dispatchEvent(new MouseEvent('pointercancel', { bubbles: true })))

    expect(trashZone().parentElement?.parentElement?.className).toContain('max-h-0')

    pointerOver(rowFor('c1'))
    pointerUp()
    expect(state().agentChats.order).toEqual([])
  })

  // ── Drag: drop-to-delete ──────────────────────────────────────────

  it('the trash zone only slides in while dragging', () => {
    seed()
    render(<AgentChatsPanel />)
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

    pointerOver(null)
    pointerDown(rowFor('c1'))
    pointerMove()
    expect(trashZone().className).toContain('border-destructive/40')

    pointerOver(trashZone())
    pointerMove()
    expect(trashZone().className).toContain('bg-destructive/10')
  })

  it('dropping a row on the trash deletes it with no confirm and closes its pane tab', async () => {
    seed()
    render(<AgentChatsPanel />)
    fireEvent.click(rowFor('c1')) // open its pane first
    expect(agentBuffers()).toHaveLength(1)

    dragOnto('c1', trashZone())

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

    dragOnto('c1', trashZone())

    // The deleted tab is gone AND the pane fell through to its sibling — not left pointing
    // at a buffer that no longer exists (which renders the pane's empty state).
    expect(state().panes[pane].bufferIds).toEqual([survivor])
    expect(state().panes[pane].activeBufferId).toBe(survivor)
    await waitFor(() => expect(deleteChatFn).toHaveBeenCalledWith('w1', 'c1'))
  })

  it('deleting a chat with no open pane tab is a no-op on the buffers', () => {
    seed()
    render(<AgentChatsPanel />)

    dragOnto('c1', trashZone())

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

    dragOnto('c1', trashZone())
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

describe('reorderIds', () => {
  it('moves the dragged id in front of the target', () => {
    expect(reorderIds(['a', 'b', 'c'], 'c', 'a')).toEqual(['c', 'a', 'b'])
    expect(reorderIds(['a', 'b', 'c'], 'a', 'c')).toEqual(['b', 'a', 'c'])
  })

  it('is a no-op when the ids match or the target is unknown', () => {
    expect(reorderIds(['a', 'b', 'c'], 'a', 'a')).toEqual(['a', 'b', 'c'])
    expect(reorderIds(['a', 'b', 'c'], 'a', 'zz')).toEqual(['a', 'b', 'c'])
  })
})
