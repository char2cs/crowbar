/**
 * The Chats panel's performance contract, as tests rather than aspirations.
 *
 * A workspace with a thousand chats has to cost what the panel costs today. Four
 * properties hold that up, and each one is a defect that has already happened
 * here at least once:
 *
 *  G1 WINDOWING HOLDS. A tree flattens to a uniform-height row array, which is
 *     exactly what a virtualizer wants — so the panel mounts the slice on screen
 *     and nothing else, however deep the tree is.
 *  G2 A TURN FRAME DOES NOT REBUILD THE TREE. `turn_started`/`turn_stopped` is
 *     the hottest event on the chat feed. The panel does not subscribe to the
 *     working map at all (immer replaces its whole reference on every frame) and
 *     the build is memoised on what the tree is MADE of.
 *  G3 ONE SPINNER RE-RENDERS ONE ROW. Each row self-subscribes to its own
 *     working state and takes only primitives and stable callbacks.
 *  G4 OPENING THE RIGHT-CLICK MENU RE-RENDERS NO ROWS. The menu is a sibling of
 *     the tree holding its own open state, because with that state inside the
 *     tree every row re-rendered to draw a popup that is not part of it.
 *  G5 ONE SEARCH KEYSTROKE AT 1000 CHATS LANDS INSIDE A FRAME.
 *
 * These deliberately use the REAL virtualizer — the other panel tests stand it
 * in with one that renders every row, which is the right trade for asserting
 * behaviour and exactly the wrong one for asserting windowing.
 */
import { act, cleanup, fireEvent, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// ── Mocks ───────────────────────────────────────────────────────────

const { streamHook, listChatFoldersFn, buildSpy } = vi.hoisted(() => ({
  streamHook: vi.fn(),
  listChatFoldersFn: vi.fn(),
  buildSpy: vi.fn(),
}))

vi.mock('@/features/agent/api/agent-api', () => ({
  createChat: vi.fn(),
  deleteChat: vi.fn(),
  renameChat: vi.fn(),
  stopChat: vi.fn(async () => {}),
  listChatFolders: (...a: unknown[]) => listChatFoldersFn(...a),
  createChatFolder: vi.fn(),
  updateChatFolder: vi.fn(),
  deleteChatFolder: vi.fn(),
  setChatPlacement: vi.fn(),
}))

vi.mock('@/features/workspace/stores/hooks/use-workspace-agent-chats-stream', () => ({
  useWorkspaceAgentChatsStream: (wsId: string) => streamHook(wsId),
}))

// Counts the builds without changing what one does. A spy on the module the
// panel imports is the only honest way to ask "did this run again?" — a render
// counter cannot tell a re-render that reused the memo from one that did not.
vi.mock('@/features/agent/lib/chat-rows', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/features/agent/lib/chat-rows')>()
  return {
    ...actual,
    buildChatTree: (input: Parameters<typeof actual.buildChatTree>[0]) => {
      buildSpy()
      return actual.buildChatTree(input)
    },
  }
})

// Each row renders its glyph exactly once, and every chat here carries a unique
// provider icon — so the glyph is a per-ROW render counter.
const rowRenders = new Map<string, number>()
vi.mock('@/features/agent/components/agent-chat-glyph', () => ({
  AgentChatGlyph: ({ providerIcon }: { providerIcon: string }) => {
    rowRenders.set(providerIcon, (rowRenders.get(providerIcon) ?? 0) + 1)
    return <span data-glyph={providerIcon} />
  },
}))

vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/editor/stores/buffer-session-persistence', () => ({
  saveSessionToStore: vi.fn(),
  clearQueuedWorkspaceSessionSave: vi.fn(),
}))

import { AgentChatsPanel } from '@/features/agent/components/agent-chats-panel'
import { buildChatTree } from '@/features/agent/lib/chat-rows'
import { AGENT_CHAT_ROW_HEIGHT } from '@/features/agent/components/use-agent-chat-list-virtualizer'
import {
  destroyWorkspaceStore,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import { setActiveWorkspaceStoreRef } from '@/features/workspace/stores/workspace-store-ref'
import type { AgentChat, AgentProvider } from '@/features/agent/api/agent-api'

// ── Fixtures ────────────────────────────────────────────────────────

const CHAT_COUNT = 1000

/** The viewport the stubbed layout reports — jsdom measures everything as zero. */
const VIEWPORT_H = 400

/**
 * A workspace with `count` chats, every fourth one a thread of the chat above
 * it: a flat list would let a virtualizer off far too lightly, and depth is the
 * thing a tree adds.
 */
function manyChats(count: number): AgentChat[] {
  const chats: AgentChat[] = []
  for (let i = 0; i < count; i++) {
    const parent = i % 4 === 0 || i === 0 ? '' : `c${i - (i % 4)}`
    chats.push({
      id: `c${i}`,
      workspaceId: 'w1',
      title: `Chat number ${i}`,
      liveRunnerId: '',
      terminalSessionId: '',
      activeProviderId: `p${i}`,
      createdAt: `2026-01-01T00:00:${String(i % 60).padStart(2, '0')}Z`,
      parentId: parent,
      order: i,
    })
  }
  return chats
}

function providersFor(chats: readonly AgentChat[]): AgentProvider[] {
  return chats.map((c) => ({
    id: c.activeProviderId,
    displayName: c.activeProviderId,
    icon: `<svg data-p="${c.activeProviderId}"></svg>`,
    connected: true,
    enabled: true,
    mcpEnabled: true,
  }))
}

const state = () => getOrCreateWorkspaceStore('w1').getState()

/** Give every element a box, so the real virtualizer has a viewport to window to. */
function stubLayout(): void {
  const rect = {
    top: 0,
    left: 0,
    right: 240,
    bottom: VIEWPORT_H,
    width: 240,
    height: VIEWPORT_H,
    x: 0,
    y: 0,
  }
  vi.spyOn(Element.prototype, 'getBoundingClientRect').mockImplementation(
    () => ({ ...rect, toJSON: () => rect }) as DOMRect,
  )
}

function seedAndRender(chats: AgentChat[]) {
  act(() => {
    setActiveWorkspaceStoreRef(getOrCreateWorkspaceStore('w1'))
    state().setAgentProviders(providersFor(chats))
    state().seedAgentChats(chats)
  })
  return render(<AgentChatsPanel />)
}

const mountedRows = () => document.querySelectorAll('[data-chat-drop],[data-chat-folder-drop]')

beforeEach(() => {
  rowRenders.clear()
  buildSpy.mockClear()
  streamHook.mockClear()
  listChatFoldersFn.mockReset().mockResolvedValue([])
  stubLayout()
})

afterEach(() => {
  cleanup()
  setActiveWorkspaceStoreRef(null)
  destroyWorkspaceStore('w1')
  vi.restoreAllMocks()
})

// ── Tests ───────────────────────────────────────────────────────────

describe('Chats panel performance gates', () => {
  it('G1: a thousand chats mount a bounded number of rows, not a thousand', () => {
    seedAndRender(manyChats(CHAT_COUNT))

    // What is on screen is what fits plus the virtualizer's overscan. The
    // absolute bound matters more than the exact number: the panel must never
    // start scaling its DOM with the chat count, whatever the viewport is.
    const onScreen = Math.ceil(VIEWPORT_H / AGENT_CHAT_ROW_HEIGHT)
    expect(mountedRows().length).toBeGreaterThan(0)
    expect(mountedRows().length).toBeLessThan(40)
    expect(mountedRows().length).toBeGreaterThanOrEqual(onScreen)
  })

  it('G1: the spacer still measures the WHOLE list, so the scrollbar tells the truth', () => {
    seedAndRender(manyChats(CHAT_COUNT))
    const spacer = document.querySelector<HTMLElement>('[data-agent-chat-scroll] > div')!
    // Every chat is a row here (the fixture nests but never folds), so the
    // spacer is the full list even though the DOM holds a couple of dozen rows.
    expect(spacer.style.height).toBe(`${CHAT_COUNT * AGENT_CHAT_ROW_HEIGHT}px`)
  })

  it('G2: a turn frame does not rebuild the tree', () => {
    seedAndRender(manyChats(CHAT_COUNT))
    const builds = buildSpy.mock.calls.length

    act(() => state().setAgentChatWorking('c0', true))
    act(() => state().setAgentChatWorking('c0', false))

    // The panel does not subscribe to the working map, and the build is memoised
    // on what the tree is made of — so the hottest event on the feed costs the
    // tree exactly nothing.
    expect(buildSpy.mock.calls.length).toBe(builds)
  })

  it('G2: a real change to the tree DOES rebuild it', () => {
    // The counter above would also pass if the build never ran at all.
    const chats = manyChats(CHAT_COUNT)
    seedAndRender(chats)
    const builds = buildSpy.mock.calls.length

    act(() => state().upsertAgentChat({ ...chats[0], id: 'brand-new', title: 'Brand new' }))

    expect(buildSpy.mock.calls.length).toBeGreaterThan(builds)
  })

  it('G3: one spinner re-renders one row', () => {
    seedAndRender(manyChats(CHAT_COUNT))
    const before = new Map(rowRenders)

    act(() => state().setAgentChatWorking('c0', true))

    // c0's row re-rendered to show the spinner. Every other MOUNTED row kept its
    // exact render count — the property that makes a busy workspace cost one row
    // per frame instead of a viewport of them.
    expect(rowRenders.get('<svg data-p="p0"></svg>')).toBe(
      (before.get('<svg data-p="p0"></svg>') ?? 0) + 1,
    )
    for (const [icon, count] of before) {
      if (icon === '<svg data-p="p0"></svg>') continue
      expect(rowRenders.get(icon)).toBe(count)
    }
  })

  it('G4: opening the right-click menu re-renders no rows', () => {
    seedAndRender(manyChats(CHAT_COUNT))
    const before = new Map(rowRenders)
    const row = document.querySelector<HTMLElement>('[data-chat-drop]')!

    fireEvent.contextMenu(row)

    // The popup is drawn — this is not passing because nothing happened.
    expect(document.querySelectorAll('[role="menuitem"]').length).toBeGreaterThan(0)
    // …and not one row was re-rendered to draw it. The menu lives beside the
    // tree and holds its own open state, so it re-renders itself.
    for (const [icon, count] of before) expect(rowRenders.get(icon)).toBe(count)
  })

  it('G5: one search keystroke at a thousand chats lands inside a frame', () => {
    const chats = manyChats(CHAT_COUNT)
    const input = {
      chats,
      folders: [],
      collapsed: new Set<string>(),
      shown: new Set<string>(),
      foldedAway: new Set<string>(),
      query: 'number 4',
    }
    // Warm the JIT, then take the BEST of several runs: the floor is the honest
    // measure of the work: any single sample can be interrupted by a GC pause
    // that says nothing about the code.
    for (let i = 0; i < 5; i++) buildChatTree(input)
    let best = Infinity
    for (let i = 0; i < 7; i++) {
      const t0 = performance.now()
      buildChatTree(input)
      best = Math.min(best, performance.now() - t0)
    }
    // The ceiling is one 60Hz frame. The target is half of it; the gate is the
    // frame, so a slow CI box cannot turn a real regression into a coin toss.
    expect(best).toBeLessThan(16)
  })
})
