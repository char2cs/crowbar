/**
 * The agent-chats sidebar panel, as a TREE: chats that hold chats, folders that
 * group them anywhere, search, kept rows, and one drag that does all of it.
 *
 * The workspace store is REAL (registry + agent-chats slice + buffer slice) so
 * placement, pane-opening and the optimistic writes are exercised end to end;
 * only the network seams (agent-api, the WS stream hook) and the IDB persistence
 * writers are mocked.
 *
 * jsdom implements neither the PointerEvent constructor nor
 * `document.elementsFromPoint`, and has no layout at all. Drags are driven with
 * MouseEvents named "pointerdown"/"pointermove"/"pointerup" (React and the
 * window listeners duck-type by name — see pane-sash.test.tsx) over a stubbed
 * layout: every rendered row is given a 40px box on a fixed pitch, and
 * elementsFromPoint answers from those boxes. That is the same harness the
 * workspace tree's drag tests use, and it exercises the SHARED hit test rather
 * than a second one written for the chats panel.
 */
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/lib/api'

// ── Mocks ───────────────────────────────────────────────────────────

const {
  createChatFn,
  deleteChatFn,
  renameChatFn,
  stopChatFn,
  listChatFoldersFn,
  createChatFolderFn,
  updateChatFolderFn,
  deleteChatFolderFn,
  setChatPlacementFn,
  streamHook,
  toastErrorFn,
} = vi.hoisted(() => ({
  createChatFn: vi.fn(),
  deleteChatFn: vi.fn(),
  renameChatFn: vi.fn(),
  // stopChat is fired by closeBuffer when a chat TAB is closed. It must be
  // exported here or the fire-and-forget dynamic import in buffer-slice throws
  // an unhandled rejection during these tests.
  stopChatFn: vi.fn(async (..._a: unknown[]) => {}),
  listChatFoldersFn: vi.fn(),
  createChatFolderFn: vi.fn(),
  updateChatFolderFn: vi.fn(),
  deleteChatFolderFn: vi.fn(),
  setChatPlacementFn: vi.fn(),
  streamHook: vi.fn(),
  toastErrorFn: vi.fn(),
}))

vi.mock('@/features/agent/api/agent-api', () => ({
  createChat: (...a: unknown[]) => createChatFn(...a),
  deleteChat: (...a: unknown[]) => deleteChatFn(...a),
  renameChat: (...a: unknown[]) => renameChatFn(...a),
  stopChat: (...a: unknown[]) => stopChatFn(...a),
  listChatFolders: (...a: unknown[]) => listChatFoldersFn(...a),
  createChatFolder: (...a: unknown[]) => createChatFolderFn(...a),
  updateChatFolder: (...a: unknown[]) => updateChatFolderFn(...a),
  deleteChatFolder: (...a: unknown[]) => deleteChatFolderFn(...a),
  setChatPlacement: (...a: unknown[]) => setChatPlacementFn(...a),
}))

vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: (...a: unknown[]) => toastErrorFn(...a) },
}))

// The removal tray at the foot of the panel reads the route to know which
// workspace the editor is showing (it navigates away when a removal takes it).
// Nothing here is on a route at all.
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => () => {},
  useRouterState: () => '',
  useRouter: () => ({ state: { location: { pathname: '/' } } }),
  useMatch: () => null,
}))

vi.mock('@/features/workspace/stores/hooks/use-workspace-agent-chats-stream', () => ({
  useWorkspaceAgentChatsStream: (wsId: string) => streamHook(wsId),
}))

// The list is virtualized; the real virtualizer measures DOM layout, which jsdom
// reports as zero — so it would render no rows. Replace it with a deterministic
// stand-in that renders EVERY row, so every assertion about rendered rows and
// drag targeting still holds. (The windowing itself is pinned for real, against
// the real library, in agent-chats-panel-perf.test.tsx.)
// `overshoot` lets a test make the windowing library report MORE items than the
// list holds — the one way the panel's own index guard can be reached.
const virtualState = vi.hoisted(() => ({ overshoot: 0 }))

vi.mock('@tanstack/react-virtual', () => ({
  useVirtualizer: (options: { count: number; estimateSize: () => number }) => {
    const size = options.estimateSize()
    return {
      getVirtualItems: () =>
        Array.from({ length: options.count + virtualState.overshoot }, (_, index) => ({
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
import { CHAT_SPRING_OPEN_MS } from '@/features/agent/components/use-agent-chats-drag'
import {
  destroyWorkspaceStore,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import { setActiveWorkspaceStoreRef } from '@/features/workspace/stores/workspace-store-ref'
import { parseWorkspaceScopeFromPath } from '@/lib/workspace-scope'
import { ROW_INDENT_STEP } from '@/components/layout/workspace-row-base'
import { EditorRemovalOverlay, PANE_ARM_MS } from '@/components/layout/editor-removal-overlay'
import {
  getInitialRemovalState,
  REMOVAL_DRAIN_MS,
  useRemovalTrayStore,
  type RemovalEntry,
} from '@/lib/store/sidebar-removal'
import { useSidebarStore } from '@/lib/store/sidebar'
import type { AgentChat, AgentChatFolder } from '@/features/agent/api/agent-api'

// ── Fixtures / helpers ──────────────────────────────────────────────

const chat = (
  id: string,
  title: string,
  providerId: string,
  createdAt: string,
  extra: { parentId?: string; order?: number; workspaceId?: string } = {},
): AgentChat => ({
  id,
  workspaceId: extra.workspaceId ?? 'w1',
  title,
  liveRunnerId: `${id}-r`,
  terminalSessionId: `${id}-pty`,
  activeProviderId: providerId,
  createdAt,
  parentId: extra.parentId ?? '',
  order: extra.order ?? 0,
})

const folder = (
  id: string,
  name: string,
  extra: { parentId?: string; order?: number } = {},
): AgentChatFolder => ({
  id,
  workspaceId: 'w1',
  name,
  parentId: extra.parentId ?? '',
  order: extra.order ?? 0,
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

/** A parent chat with two threads, one of them holding a thread of its own. */
const THREADED = [
  chat('p1', 'Parent', 'claude', '2026-01-01T00:00:00Z'),
  chat('t1', 'Thread one', 'claude', '2026-01-02T00:00:00Z', { parentId: 'p1', order: 0 }),
  chat('t2', 'Thread two', 'codex', '2026-01-03T00:00:00Z', { parentId: 'p1', order: 1 }),
  chat('t2a', 'Deep thread', 'codex', '2026-01-04T00:00:00Z', { parentId: 't2', order: 0 }),
]

const THREE = [
  chat('c1', 'First', 'claude', '2026-01-01T00:00:00Z', { order: 0 }),
  chat('c2', 'Second', 'codex', '2026-01-02T00:00:00Z', { order: 1 }),
  chat('c3', 'Third', 'claude', '2026-01-03T00:00:00Z', { order: 2 }),
]

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

/**
 * Seed providers, chats and folders.
 *
 * The folders go into the store AND into the GET the panel's own seeding hook
 * fires, so the hook's answer landing a microtask later re-writes exactly what
 * is already there instead of wiping it.
 */
function seed(chats: AgentChat[] = [CHAT_1, CHAT_2], folders: AgentChatFolder[] = [], wsId = 'w1') {
  listChatFoldersFn.mockResolvedValue(folders)
  const st = state(wsId)
  act(() => {
    st.setAgentProviders(PROVIDERS)
    for (const c of chats) st.upsertAgentChat(c)
    st.seedAgentChatFolders(folders)
  })
}

const CHAT_SELECTOR = '[data-chat-drop],[data-chat-folder-drop]'

/**
 * Every row on screen, top to bottom, chats and folders alike.
 *
 * The ghost is excluded: it carries CLONES of the grabbed rows, attributes and
 * all, so a mid-drag query would count the row it is carrying twice.
 */
function rowEls(): HTMLElement[] {
  return [...document.querySelectorAll<HTMLElement>(CHAT_SELECTOR)].filter(
    (el) => el.closest('[data-drag-ghost]') === null,
  )
}

/**
 * Every row on screen BY LABEL, kept rows included.
 *
 * A row hoisted out of a folded ancestor publishes no drop attributes at all —
 * it is not drawn where it lives — so it is invisible to rowIds() by design.
 */
function rowLabels(): string[] {
  return [...document.querySelectorAll<HTMLElement>('[role="treeitem"]')]
    .filter((el) => el.closest('[data-drag-ghost]') === null)
    .map((el) => el.textContent?.trim() ?? '')
}

function rowIds(): string[] {
  return rowEls().map(
    (el) => el.getAttribute('data-chat-drop') ?? el.getAttribute('data-chat-folder-drop') ?? '',
  )
}

function rowFor(id: string): HTMLElement {
  const el = document.querySelectorAll<HTMLElement>(
    `[data-chat-drop="${id}"],[data-chat-folder-drop="${id}"]`,
  )
  const live = [...el].find((node) => node.closest('[data-drag-ghost]') === null)
  if (!live) throw new Error(`no row for ${id}`)
  return live
}

/** A row's indent, in ROW_INDENT_STEP units — the tree's only depth signal. */
function depthOf(id: string): number {
  const indent = rowFor(id).parentElement!.style.marginInlineStart
  return parseInt(indent || '0', 10) / ROW_INDENT_STEP
}

/** A row found by the text it shows — the only handle a kept row offers. */
function keptRow(label: string): HTMLElement {
  const el = [...document.querySelectorAll<HTMLElement>('[role="treeitem"]')].find(
    (node) => node.textContent?.trim() === label,
  )
  if (!el) throw new Error(`no row labelled ${label}`)
  return el
}

/**
 * The panel, and the editor pane beside it.
 *
 * The pane is where a row goes to be REMOVED — the same target, the same veil
 * and the same eight-second tray as a workspace row — so it has to be on screen
 * for any of that to be reachable. In the real app it is ide-shell.tsx's content
 * pane; here it is the smallest thing that carries the same two facts.
 */
function Shell({ wsId }: { wsId?: string } = {}) {
  return (
    <>
      <AgentChatsPanel wsId={wsId} />
      <div data-pane-drop="">
        <EditorRemovalOverlay />
      </div>
    </>
  )
}

const overlay = () => document.querySelector<HTMLElement>('[data-pane-removal]')!
/** The zone is DRAWN — up for the whole drag, in either of its two states. */
const veilUp = () => !overlay().hidden
/** A release right now would remove — the state the dwell unlocks. */
const veilArmed = () => overlay().hasAttribute('data-armed')
const veilTitle = () => overlay().querySelector('[data-pane-removal-title]')!.textContent
const veilDetail = () => overlay().querySelector('[data-pane-removal-detail]')!.textContent

/** The rows the removal tray is holding, oldest first. */
const trayIds = () => useRemovalTrayStore.getState().entries.map((e: RemovalEntry) => e.id)
/** The tray's own rows, as drawn at the foot of the panel. */
const trayRows = () =>
  [...document.querySelectorAll<HTMLElement>('[data-removal-entry]')].map((el) =>
    el.textContent?.trim(),
  )

function pointerDown(el: HTMLElement, button = 0, x = 10, y = 0) {
  act(() => {
    el.dispatchEvent(
      new MouseEvent('pointerdown', { button, clientX: x, clientY: y, bubbles: true }),
    )
  })
}

function pointerMove(x = 10, y = 0) {
  act(() => {
    window.dispatchEvent(new MouseEvent('pointermove', { clientX: x, clientY: y, bubbles: true }))
  })
}

function pointerUp(x = 10, y = 0) {
  act(() => {
    window.dispatchEvent(new MouseEvent('pointerup', { clientX: x, clientY: y, bubbles: true }))
  })
}

const ROW_H = 40
/** Anything at or beyond this x is the editor pane; the panel is to its left. */
const PANE_X = 400

function stubRect(el: Element | null, rect: Partial<DOMRect>) {
  if (!el) throw new Error('stubRect: element not found')
  const full = { top: 0, left: 0, right: 0, bottom: 0, width: 0, height: 0, x: 0, y: 0, ...rect }
  el.getBoundingClientRect = () => ({ ...full, toJSON: () => full }) as DOMRect
}

/**
 * Give the rendered rows a layout and point the hit test at it.
 *
 * Rows sit on the virtualizer's fixed 40px pitch from the top of the scroll
 * container, which is where the panel really draws them. Re-run it after
 * anything that adds or removes rows — a row that has never been measured
 * reports a zero box and the hit test steps over it.
 */
function layout() {
  rowEls().forEach((el, i) =>
    stubRect(el, {
      top: i * ROW_H,
      bottom: (i + 1) * ROW_H,
      left: 0,
      right: 200,
      width: 200,
      height: ROW_H,
    }),
  )
  stubRect(document.querySelector('[data-agent-chat-scroll]'), {
    top: 0,
    left: 0,
    right: 200,
    bottom: 1200,
    width: 200,
    height: 1200,
  })
  document.elementsFromPoint = ((x: number, y: number) => {
    const pane = document.querySelector<HTMLElement>('[data-pane-drop]')
    if (pane && x >= PANE_X) return [pane]
    for (const el of rowEls()) {
      const r = el.getBoundingClientRect()
      if (r.height > 0 && y >= r.top && y < r.bottom) return [el]
    }
    return []
  }) as typeof document.elementsFromPoint
}

/**
 * Where in a row to aim.
 *
 * The bands are the shared ones: 30% at each end of a CHAT (re-parenting one
 * rewrites what it reads, so the expensive move is the harder target) and 20% on
 * a folder. `top`/`bottom` land inside both; `middle` nests in either.
 */
type Aim = 'top' | 'middle' | 'bottom'

function aimAt(id: string, where: Aim): number {
  const r = rowFor(id).getBoundingClientRect()
  if (where === 'top') return r.top + 2
  if (where === 'bottom') return r.bottom - 2
  return r.top + ROW_H / 2
}

/** Grab `dragId` and hold the pointer over `targetId`, without letting go. */
function dragOver(dragId: string, targetId: string, where: Aim) {
  layout()
  const from = rowFor(dragId).getBoundingClientRect()
  pointerDown(rowFor(dragId), 0, 10, from.top + 20)
  pointerMove(10, from.top + 30) // crosses the 5px threshold → the drag goes live
  pointerMove(10, aimAt(targetId, where))
}

/** The whole gesture: grab `dragId`, drop it on `targetId`. */
function dragOnto(dragId: string, targetId: string, where: Aim) {
  dragOver(dragId, targetId, where)
  pointerUp(10, aimAt(targetId, where))
}

/**
 * Drag `dragId` onto the editor pane and hold there for `ms` before letting go.
 *
 * The dwell is the guard: a long reorder crosses this pane on its way, so a
 * release that has not waited it out is a drag that ended in the wrong place.
 */
function dragToPane(dragId: string, ms = PANE_ARM_MS) {
  layout()
  const from = rowFor(dragId).getBoundingClientRect()
  pointerDown(rowFor(dragId), 0, 10, from.top + 20)
  pointerMove(10, from.top + 30)
  pointerMove(PANE_X, 100)
  act(() => {
    vi.advanceTimersByTime(ms)
  })
  pointerUp(PANE_X, 100)
  // A drop arms a one-shot capture-phase click trap (the post-drop click must
  // not select the row it landed on), dropped on the next macrotask exactly as
  // the browser would. Under fake timers nothing drops it, so without this the
  // next click in the test — Keep, a menu item — is silently swallowed.
  act(() => {
    vi.advanceTimersByTime(1)
  })
}

/** Drag `dragId` onto the pane, hold it there, and DON'T let go. */
function dragOverPane(dragId: string, ms = PANE_ARM_MS) {
  layout()
  const from = rowFor(dragId).getBoundingClientRect()
  pointerDown(rowFor(dragId), 0, 10, from.top + 20)
  pointerMove(10, from.top + 30)
  pointerMove(PANE_X, 100)
  act(() => {
    vi.advanceTimersByTime(ms)
  })
}

/** The one hairline the whole drag shares, portalled to the body. */
const dropLine = () => document.querySelector<HTMLElement>('[data-drop-indicator]')
const ghost = () => document.querySelector<HTMLElement>('[data-drag-ghost]')

const agentBuffers = () => state().buffers.filter((b) => b.type === 'agentChat')

/** Every placement PATCH the drag fired, in the order it fired them. */
const placements = () =>
  setChatPlacementFn.mock.calls.map(
    (c) => [c[1], (c[2] as { parentId?: string }).parentId] as const,
  )

beforeEach(() => {
  virtualState.overshoot = 0
  localStorage.clear()
  // The tray is a GLOBAL store shared with the workspace sidebar; a row left
  // held by one test would hide it in the next.
  useRemovalTrayStore.setState(getInitialRemovalState())
  // Folded rows are a GLOBAL store too, and deliberately so — they outlive the
  // workspace switch that remounts this panel. A row left folded by one test
  // would start the next one folded.
  useSidebarStore.setState({ collapsedChatRows: new Set<string>() })
  createChatFn.mockReset().mockResolvedValue('c-new')
  deleteChatFn.mockReset().mockResolvedValue(undefined)
  renameChatFn.mockReset().mockResolvedValue(undefined)
  listChatFoldersFn.mockReset().mockResolvedValue([])
  createChatFolderFn.mockReset()
  updateChatFolderFn.mockReset().mockImplementation(async (_ws: string, id: string, patch) => ({
    folder: {
      ...(state().agentChats.folders.find((f) => f.id === id) as AgentChatFolder),
      ...patch,
    },
    shifted: [],
  }))
  deleteChatFolderFn.mockReset().mockResolvedValue([])
  setChatPlacementFn.mockReset().mockImplementation(async (_ws: string, id: string, patch) => {
    const c = state().agentChats.chats.find((x) => x.id === id) as AgentChat
    return { chat: { ...c, ...patch }, shifted: [] }
  })
  toastErrorFn.mockReset()
  streamHook.mockClear()
  // jsdom implements no pointer capture, and the drag takes it the moment the
  // threshold is crossed. Assigned rather than spied: on this platform there is
  // no original method for a spy to wrap.
  Element.prototype.setPointerCapture = () => {}
  // The drag's edge auto-scroll runs in a rAF loop; jsdom cannot scroll and the
  // loop has its own unit tests, so make rAF inert here (returns an id, never
  // invokes the callback).
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
  // not select the dragged row); the drag drops it on the next macrotask, as the
  // browser would. Drain that already-scheduled task so an unconsumed trap never
  // leaks into the next test.
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
    render(<Shell />)
    expect(document.querySelector('[data-agent-chat-scroll]')).toBeNull()
    expect(streamHook).not.toHaveBeenCalled()
  })

  it('mounts the agent-chats WS stream for the active workspace', () => {
    seed()
    render(<Shell />)
    expect(streamHook).toHaveBeenCalledWith('w1')
  })

  it('reads this workspace’s folders once, and never polls for them', async () => {
    seed()
    render(<Shell />)
    await act(async () => {})
    expect(listChatFoldersFn).toHaveBeenCalledTimes(1)
    expect(listChatFoldersFn).toHaveBeenCalledWith('w1')
  })

  // The project-home route is /ide/:projectId/home: no wsId in the URL (it is
  // resolved asynchronously and published by WorkspaceView). Deriving wsId from
  // the pathname left the Chats tab blank on every home workspace.
  it('renders chats + New rows on a project-home workspace (no wsId in the URL)', () => {
    expect(parseWorkspaceScopeFromPath('/ide/p1/home')).toBeNull()

    activate('hw1')
    seed(
      [chat('h1', 'Home chat', 'claude', '2026-03-01T00:00:00Z', { workspaceId: 'hw1' })],
      [],
      'hw1',
    )
    const { container } = render(<Shell />)

    expect(streamHook).toHaveBeenCalledWith('hw1')
    expect(rowIds()).toEqual(['h1'])
    expect(screen.getByText('Home chat')).toBeTruthy()
    // One unified New-chat row, not one-per-provider.
    expect(container.querySelectorAll('[data-new-chat]')).toHaveLength(1)
  })

  it('follows the active workspace when it changes (worktree → project home)', () => {
    seed()
    seed(
      [chat('h1', 'Home chat', 'claude', '2026-03-01T00:00:00Z', { workspaceId: 'hw1' })],
      [],
      'hw1',
    )
    render(<Shell />)
    expect(rowIds()).toEqual(['c2', 'c1'])

    activate('hw1')

    expect(rowIds()).toEqual(['h1'])
    expect(streamHook).toHaveBeenLastCalledWith('hw1')
  })

  it('renders for an explicit wsId prop (the sidebar host may pass one)', () => {
    act(() => setActiveWorkspaceStoreRef(null))
    seed()
    render(<Shell wsId="w1" />)
    expect(rowIds()).toEqual(['c2', 'c1'])
  })

  it('orders siblings by the server’s own order field', () => {
    // Placement is domain truth now, not a browser key: the daemon hands back an
    // `order` per row and the tree sorts on it. c1 is the OLDER chat and still
    // leads, which recency alone would never produce.
    seed([
      chat('c1', 'First', 'claude', '2026-01-01T00:00:00Z', { order: 0 }),
      chat('c2', 'Second', 'codex', '2026-01-02T00:00:00Z', { order: 1 }),
    ])
    render(<Shell />)
    expect(rowIds()).toEqual(['c1', 'c2'])
  })

  it('breaks an order tie by recency — the arrangement a daemon that has placed nothing gives', () => {
    // Every chat arrives at order 0 until something is dragged, and a list that
    // buried the chat you just started was the defect that rule exists to avoid.
    seed()
    render(<Shell />)
    expect(rowIds()).toEqual(['c2', 'c1'])
  })

  it('resolves each row provider icon from the providers list', () => {
    seed()
    render(<Shell />)
    expect(rowFor('c1').querySelector('[data-p="claude"]')).not.toBeNull()
    expect(rowFor('c2').querySelector('[data-p="codex"]')).not.toBeNull()
  })

  it('falls back to the chat glyph for a chat whose provider is unknown', () => {
    seed([chat('c9', 'Orphan', 'gemini', '2026-01-03T00:00:00Z')])
    render(<Shell />)
    // Not the provider's icon (there isn't one) and NOT an empty slot: a chat with
    // no resolvable provider still has to read as a chat — specifically the chat
    // glyph, not merely some svg (a file icon would satisfy that and defeat the
    // fallback's point).
    expect(rowFor('c9').querySelector('[data-provider-icon]')).toBeNull()
    expect(rowFor('c9').querySelector('[data-chat-glyph]')).not.toBeNull()
  })

  it('shows the working spinner on the working chat only', () => {
    seed()
    act(() => state().setAgentChatWorking('c2', true))
    render(<Shell />)
    expect(rowFor('c2').querySelector('[role="status"]')).not.toBeNull()
    expect(rowFor('c1').querySelector('[role="status"]')).toBeNull()
  })

  it('clicking a chat row selects it and opens its agentChat pane', () => {
    seed()
    render(<Shell />)
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
    render(<Shell />)
    fireEvent.click(rowFor('c3'))
    expect(agentBuffers()[0]).toMatchObject({ chatId: 'c3', name: 'Untitled chat' })
  })

  it('marks the active chat row active', () => {
    seed()
    render(<Shell />)
    fireEvent.click(rowFor('c2'))
    expect(rowFor('c2').className).toContain('bg-background')
    expect(rowFor('c1').className).toContain('hover:bg-accent')
  })

  // ── The tree ──────────────────────────────────────────────────────

  it('draws a thread as an indent under its parent — no badge, no count', () => {
    seed(THREADED)
    render(<Shell />)

    expect(rowIds()).toEqual(['p1', 't1', 't2', 't2a'])
    expect(depthOf('p1')).toBe(0)
    expect(depthOf('t1')).toBe(1)
    expect(depthOf('t2a')).toBe(2)
    // The indent is the ENTIRE statement of the relationship. An earlier
    // iteration drew a fork badge; it described a snapshot that is not taken.
    expect(rowFor('p1').textContent).toBe('Parent')
  })

  it('collapses a parent and hides everything under it', () => {
    seed(THREADED)
    render(<Shell />)

    fireEvent.click(screen.getAllByRole('button', { name: /collapse/i })[0])

    expect(rowIds()).toEqual(['p1'])
    fireEvent.click(screen.getAllByRole('button', { name: /expand/i })[0])
    expect(rowIds()).toEqual(['p1', 't1', 't2', 't2a'])
  })

  // Reported after the tree shipped: every folded folder sprang open on the way
  // back to a workspace. The panel is keyed by workspace id, so the switch
  // remounts it — and what the user had folded lived in component state, which
  // the remount threw away. The fold now outlives the panel.
  it('keeps folded rows folded across a workspace switch and back', () => {
    seed(
      [CHAT_1, chat('cf', 'Filed', 'claude', '2026-01-05T00:00:00Z', { parentId: 'f1' })],
      [folder('f1', 'Spikes')],
    )
    seed(
      [chat('h1', 'Home chat', 'claude', '2026-03-01T00:00:00Z', { workspaceId: 'hw1' })],
      [],
      'hw1',
    )
    render(<Shell />)
    expect(rowIds()).toEqual(['f1', 'cf', 'c1'])

    fireEvent.click(screen.getAllByRole('button', { name: /collapse/i })[0])
    expect(rowIds()).toEqual(['f1', 'c1'])

    activate('hw1')
    expect(rowIds()).toEqual(['h1'])

    activate('w1')
    expect(rowIds()).toEqual(['f1', 'c1'])
    // And it is still a FOLD, not a row that lost its control: it opens again.
    fireEvent.click(screen.getAllByRole('button', { name: /expand/i })[0])
    expect(rowIds()).toEqual(['f1', 'cf', 'c1'])
  })

  // The other half of the keyed remount: a fold survives it, a SELECTION must
  // not. Asserted here because both now hang off the same switch.
  it('drops the selection across a workspace switch even though the fold survives', () => {
    seed(
      [CHAT_1, chat('cf', 'Filed', 'claude', '2026-01-05T00:00:00Z', { parentId: 'f1' })],
      [folder('f1', 'Spikes')],
    )
    seed(
      [chat('h1', 'Home chat', 'claude', '2026-03-01T00:00:00Z', { workspaceId: 'hw1' })],
      [],
      'hw1',
    )
    render(<Shell />)

    fireEvent.click(screen.getAllByRole('button', { name: /collapse/i })[0])
    fireEvent.click(rowFor('c1'), { metaKey: true })
    expect(rowFor('c1').getAttribute('aria-selected')).toBe('true')

    activate('hw1')
    activate('w1')

    expect(rowIds()).toEqual(['f1', 'c1'])
    expect(rowFor('c1').getAttribute('aria-selected')).toBe('false')
  })

  it('renders a folder’s contents under it, and folders sort above chats at the same order', () => {
    seed(
      [CHAT_1, chat('cf', 'Filed', 'claude', '2026-01-05T00:00:00Z', { parentId: 'f1' })],
      [folder('f1', 'Spikes')],
    )
    render(<Shell />)

    expect(rowIds()).toEqual(['f1', 'cf', 'c1'])
    expect(depthOf('cf')).toBe(1)
    expect(screen.getByText('Spikes')).toBeTruthy()
  })

  it('files a folder inside a CHAT without changing what its threads read', () => {
    // A folder holds no turns, so lineage steps straight through it: a thread two
    // folders deep under a chat has exactly the chat ancestry of one sitting
    // directly under it. That is the property that lets folders go anywhere.
    seed(
      [
        chat('p1', 'Parent', 'claude', '2026-01-01T00:00:00Z'),
        chat('direct', 'Direct thread', 'claude', '2026-01-02T00:00:00Z', { parentId: 'p1' }),
        chat('filed', 'Filed thread', 'claude', '2026-01-03T00:00:00Z', { parentId: 'f2' }),
      ],
      [folder('f1', 'Group', { parentId: 'p1' }), folder('f2', 'Inner', { parentId: 'f1' })],
    )
    render(<Shell />)

    // Both threads' CHAT ancestry is exactly [p1] — the folders in between are
    // published in the path (they are containers) but carry no conversation.
    const chatAncestors = (id: string) =>
      (rowFor(id).getAttribute('data-chat-path') ?? '')
        .split('/')
        .filter(Boolean)
        .filter((ancestor) => document.querySelector(`[data-chat-drop="${ancestor}"]`) !== null)
    expect(chatAncestors('direct')).toEqual(['p1'])
    expect(chatAncestors('filed')).toEqual(['p1'])
  })

  it('renders a chat whose parent is gone at the root rather than nowhere', () => {
    // A row that exists in the store and nowhere on screen is the worst failure
    // this panel has, and a parent's delete frame arriving first is ordinary.
    seed([chat('orphan', 'Orphan', 'claude', '2026-01-01T00:00:00Z', { parentId: 'ghost' })])
    render(<Shell />)
    expect(rowIds()).toEqual(['orphan'])
    expect(depthOf('orphan')).toBe(0)
  })

  // ── Kept rows ─────────────────────────────────────────────────────

  it('a folded parent keeps the chat you are looking at, hoisted one step in', () => {
    seed(THREADED)
    render(<Shell />)
    fireEvent.click(rowFor('t2a')) // on screen now

    fireEvent.click(screen.getAllByRole('button', { name: /collapse/i })[0])

    // p1 is folded, but what you are READING is still there — one step under the
    // row that is holding it, whatever depth it really lives at.
    expect(rowLabels()).toEqual(['Parent', 'Deep thread'])
    expect(keptRow('Deep thread').parentElement!.style.marginInlineStart).toBe(
      `${ROW_INDENT_STEP}px`,
    )
    // …and the row holding it says so, inside its own glyph.
    expect(rowFor('p1').querySelector('[data-holding-rows]')).not.toBeNull()
  })

  it('the fold-away control lets the kept rows go, without opening the parent', () => {
    seed(THREADED)
    render(<Shell />)
    fireEvent.click(rowFor('t2a'))
    fireEvent.click(screen.getAllByRole('button', { name: /collapse/i })[0])

    fireEvent.click(screen.getByRole('button', { name: /fold away/i }))

    expect(rowLabels()).toEqual(['Parent'])
    // Still folded — folding away is not the same gesture as opening.
    expect(rowFor('p1').getAttribute('aria-expanded')).toBe('false')
  })

  it('re-opening a folded row forgets the fold-away it was carrying', () => {
    seed(THREADED)
    render(<Shell />)
    fireEvent.click(rowFor('t2a'))
    fireEvent.click(screen.getAllByRole('button', { name: /collapse/i })[0])
    fireEvent.click(screen.getByRole('button', { name: /fold away/i }))

    fireEvent.click(screen.getByRole('button', { name: /expand/i }))
    fireEvent.click(screen.getAllByRole('button', { name: /collapse/i })[0])

    // The kept row is back: a dismissal belongs to the fold it was made in.
    expect(rowLabels()).toEqual(['Parent', 'Deep thread'])
  })

  // ── A kept row is the SAME row ────────────────────────────────────
  //
  // The sidebar's rule, verbatim (sidebar-tree-node.tsx): a row a folded parent
  // is still showing "carries no treatment of its own — it is simply still on
  // screen", and the drag keeps reading its REAL container rather than the
  // parent that hoisted it. It was drawn inert here once, which made the one
  // chat you are looking at the one chat you cannot drag out of the folder that
  // swallowed it.

  /** Open `chatId`, then fold the row at the top of the tree over it. */
  function foldOver(chatId: string) {
    fireEvent.click(rowFor(chatId))
    fireEvent.click(screen.getAllByRole('button', { name: /collapse/i })[0])
  }

  it('a kept row publishes its REAL container and chain, not its holder’s', () => {
    seed(THREADED)
    render(<Shell />)
    foldOver('t2a')

    expect(rowIds()).toEqual(['p1', 't2a'])
    const kept = rowFor('t2a')
    expect(kept.getAttribute('data-chat-drop')).toBe('t2a')
    // Its own parent and its own chain — 't2a' lives under 't2' under 'p1', and
    // publishing the holder's '/p1/' instead would let a drop plan against a
    // container it is not in.
    expect(kept.getAttribute('data-chat-parent')).toBe('t2')
    expect(kept.getAttribute('data-chat-path')).toBe('/p1/t2/')
    // Only the DEPTH is the holder's: exactly one step under it, whatever depth
    // the row really lives at (t2a is really two levels down).
    expect(depthOf('t2a')).toBe(1)
  })

  it('a kept row can be dragged out of the row holding it', async () => {
    seed(THREADED)
    render(<Shell />)
    foldOver('t2a')

    dragOnto('t2a', 'p1', 'top')

    // Planned against where it LIVES: out of t2, to the root, before p1.
    await waitFor(() =>
      expect(setChatPlacementFn).toHaveBeenCalledWith('w1', 't2a', { parentId: '', order: 0 }),
    )
  })

  it('a kept row is a drop target like any other', async () => {
    seed([...THREADED, chat('solo', 'Solo', 'claude', '2026-02-01T00:00:00Z', { order: 1 })])
    render(<Shell />)
    foldOver('t2a')
    expect(rowIds()).toEqual(['p1', 't2a', 'solo'])

    dragOnto('solo', 't2a', 'middle')

    await waitFor(() =>
      expect(setChatPlacementFn).toHaveBeenCalledWith('w1', 'solo', { parentId: 't2a', order: 0 }),
    )
  })

  it('still refuses the HOLDER into the row it is holding', () => {
    seed(THREADED)
    render(<Shell />)
    foldOver('t2a')

    dragOnto('p1', 't2a', 'middle')

    // t2a publishes '/p1/t2/', so this is a row into its own descendant — the
    // move that detaches a subtree from the tree entirely. It is refused because
    // the chain on the row is the row's OWN; the holder's chain would not
    // contain 'p1' and this drop would have gone through.
    expect(setChatPlacementFn).not.toHaveBeenCalled()
  })

  it('a kept row joins the ⌘-click multiselection and travels with it', async () => {
    seed([...THREADED, chat('solo', 'Solo', 'claude', '2026-02-01T00:00:00Z', { order: 1 })])
    render(<Shell />)
    foldOver('t2a')

    fireEvent.click(rowFor('t2a'), { metaKey: true })
    fireEvent.click(rowFor('solo'), { metaKey: true })
    dragOnto('solo', 'p1', 'top')

    // Both rows moved, each from its own real container.
    await waitFor(() =>
      expect(placements()).toEqual([
        ['t2a', ''],
        ['solo', ''],
      ]),
    )
  })

  it('a FOLDER can hold a row, and stays fully addressable while it does', async () => {
    seed(
      [
        chat('inside', 'Filed', 'claude', '2026-01-05T00:00:00Z', { parentId: 'f1' }),
        chat('solo', 'Solo', 'claude', '2026-01-06T00:00:00Z', { order: 1 }),
      ],
      [folder('f1', 'Spikes')],
    )
    render(<Shell />)
    await act(async () => {})
    foldOver('inside')

    // The kept chat, hoisted out of the folder, with the folder as its real
    // container.
    expect(rowIds()).toEqual(['f1', 'inside', 'solo'])
    expect(rowFor('inside').getAttribute('data-chat-parent')).toBe('f1')
    expect(rowFor('inside').getAttribute('data-chat-path')).toBe('/f1/')
    expect(depthOf('inside')).toBe(1)

    // …and the HOLDER is still a drop target while it is holding. Order 1, after
    // the chat it is already holding: a drop lands among the folder's REAL
    // contents, not among the rows a fold happens to be drawing.
    dragOnto('solo', 'f1', 'middle')
    await waitFor(() =>
      expect(setChatPlacementFn).toHaveBeenCalledWith('w1', 'solo', { parentId: 'f1', order: 1 }),
    )
  })

  it('a kept row draws no chevron — its own subtree is not hoisted with it', () => {
    seed(THREADED)
    render(<Shell />)
    // t2 holds t2a, so opening t2 and folding p1 over it hoists a row that
    // really has a child.
    fireEvent.click(rowFor('t2'))
    fireEvent.click(screen.getAllByRole('button', { name: /collapse/i })[0])

    expect(rowIds()).toEqual(['p1', 't2'])
    expect(rowFor('t2')).not.toHaveAttribute('data-chat-children')
    // The control would expand nothing: the kept row is hoisted alone.
    expect(within(rowFor('t2')).queryByLabelText(/Expand|Collapse/)).toBeNull()
  })

  it('a kept row offers its own "+", and the new thread is hoisted beside it', async () => {
    seed(THREADED)
    render(<Shell />)
    foldOver('t2a')

    fireEvent.click(screen.getByRole('button', { name: 'New thread in Deep thread' }))

    await waitFor(() => expect(createChatFn).toHaveBeenCalledWith('w1', 'claude', 't2a'))
    expect(setChatPlacementFn).not.toHaveBeenCalled()
    // The new thread is OPENED, so it is what is on screen now — and a row is
    // kept because it is on screen. It takes the hoisted slot from the chat it
    // replaced in the pane rather than being buried inside the folded row.
    act(() =>
      state().upsertAgentChat(
        chat('c-new', 'Newer', 'claude', '2026-03-01T00:00:00Z', { parentId: 't2a' }),
      ),
    )
    expect(rowIds()).toEqual(['p1', 'c-new'])
    expect(depthOf('c-new')).toBe(1)
  })

  // ── A row is lit by what is ON SCREEN, not by having a tab ────────

  it('lights the chat on screen and darkens the one behind it in the same pane', () => {
    seed()
    render(<Shell />)

    fireEvent.click(rowFor('c1'))
    fireEvent.click(rowFor('c2'))

    // Both have tabs, ONE pane, so only the active tab is on screen. The old
    // rule lit both — a chat sitting behind another stayed lit with nothing on
    // screen to justify it.
    expect(rowFor('c2').className).toContain('bg-background')
    expect(rowFor('c1').className).toContain('hover:bg-accent')

    // Bring c1 back to the front of the same pane and the lighting swaps.
    fireEvent.click(rowFor('c1'))
    expect(rowFor('c1').className).toContain('bg-background')
    expect(rowFor('c2').className).toContain('hover:bg-accent')
  })

  it('lights both chats when each is the active tab of its own pane', () => {
    seed()
    render(<Shell />)

    fireEvent.click(rowFor('c1'))
    act(() => {
      state().paneActions.splitPane(state().activePaneId, 'horizontal', 'after')
    })
    fireEvent.click(rowFor('c2'))

    expect(rowFor('c1').className).toContain('bg-background')
    expect(rowFor('c2').className).toContain('bg-background')
  })

  // ── Search ────────────────────────────────────────────────────────

  const searchField = () => screen.getByRole('textbox', { name: /search chats/i })

  it('filters the list to matching titles', async () => {
    seed()
    render(<Shell />)
    expect(rowIds()).toEqual(['c2', 'c1'])

    await userEvent.type(searchField(), 'Second')
    expect(rowIds()).toEqual(['c2'])
  })

  it('keeps a hit’s ancestors on screen, dimmed, so it never loses its parent chat', async () => {
    seed(THREADED)
    render(<Shell />)

    await userEvent.type(searchField(), 'Deep')

    expect(rowIds()).toEqual(['p1', 't2', 't2a'])
    // Scaffolding, not results: on screen so the hit can be placed, dimmed so it
    // is not mistaken for one.
    expect(rowFor('p1').className).toContain('opacity-45')
    expect(rowFor('t2a').className).not.toContain('opacity-45')
  })

  it('shows a matched row’s own threads in full', async () => {
    seed(THREADED)
    render(<Shell />)
    await userEvent.type(searchField(), 'Thread two')
    // A matched parent is usually what you were looking for, and its threads are
    // the answer.
    expect(rowIds()).toEqual(['p1', 't2', 't2a'])
  })

  it('ignores collapse while a query is active — a folded parent must not hide the only hit', async () => {
    seed(THREADED)
    render(<Shell />)
    fireEvent.click(screen.getAllByRole('button', { name: /collapse/i })[0])
    expect(rowIds()).toEqual(['p1'])

    await userEvent.type(searchField(), 'Deep')
    expect(rowIds()).toEqual(['p1', 't2', 't2a'])
  })

  it('matches folder names too, and brings what is inside them', async () => {
    seed([CHAT_1], [folder('f1', 'Firstish')])
    render(<Shell />)
    await userEvent.type(searchField(), 'Firs')
    expect(rowIds()).toEqual(['f1', 'c1'])
  })

  it('restores the whole list on escape', async () => {
    seed()
    render(<Shell />)

    await userEvent.type(searchField(), 'Second')
    expect(rowIds()).toEqual(['c2'])

    searchField().focus()
    await userEvent.keyboard('{Escape}')
    expect(rowIds()).toEqual(['c2', 'c1'])
  })

  it('renders no rows at all when nothing matches', async () => {
    seed()
    render(<Shell />)
    await userEvent.type(searchField(), 'zzzz')
    expect(rowIds()).toEqual([])
    // Nothing is drawn under the field to say so — the count line is gone.
    expect(screen.queryByTestId('chat-search-meta')).toBeNull()
  })

  it('marks the matched substring on the row, keeping its casing', async () => {
    seed()
    render(<Shell />)
    await userEvent.type(searchField(), 'seco')
    const mark = rowFor('c2').querySelector('mark')
    expect(mark).not.toBeNull()
    expect(mark!.textContent).toBe('Seco')
  })

  it('search is case-insensitive', async () => {
    seed()
    render(<Shell />)
    await userEvent.type(searchField(), 'SECOND')
    expect(rowIds()).toEqual(['c2'])
  })

  it('hides the New-chat separator when a search empties the list', async () => {
    seed()
    render(<Shell />)
    expect(document.querySelector('[data-new-chat-separator]')).not.toBeNull()
    await userEvent.type(searchField(), 'zzzz')
    expect(document.querySelector('[data-new-chat-separator]')).toBeNull()
  })

  it('drops a windowed index past the end of the list instead of crashing', () => {
    // The virtualizer is third-party and driven by rows.length; if it ever hands
    // back an index the list does not have, the sidebar must skip that row rather
    // than throw. Nothing else in the panel can produce this state.
    seed()
    virtualState.overshoot = 2
    render(<Shell />)
    expect(rowIds()).toEqual(['c2', 'c1'])
  })

  it('closing the tab puts the row out', () => {
    seed()
    render(<Shell />)

    fireEvent.click(rowFor('c1'))
    expect(rowFor('c1').className).toContain('bg-background')

    const c1Buffer = agentBuffers().find((b) => b.chatId === 'c1')!
    act(() => state().bufferActions.closeBuffer(c1Buffer.id))

    expect(rowFor('c1').className).toContain('hover:bg-accent')
  })

  it('clicking a chat already open in ANOTHER pane reveals that pane, never moving the tab', () => {
    seed()
    render(<Shell />)

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
    const { container } = render(<Shell />)

    // One unified row — not one-per-provider. It opens the FIRST ENABLED provider
    // (claude here), so the row identifies that provider, and sits after the chats.
    const rows = Array.from(container.querySelectorAll('[data-chat-drop], [data-new-chat]'))
    expect(
      rows.map((r) => r.getAttribute('data-chat-drop') ?? r.getAttribute('data-new-chat')),
    ).toEqual(['c2', 'c1', 'claude'])

    // A single, provider-agnostic label — no more "New Claude chat"/"New Codex chat".
    expect(screen.getAllByText('New chat')).toHaveLength(1)
    expect(screen.queryByText('New Claude chat')).toBeNull()

    // Separator sits above the row while there are chats to divide from.
    expect(container.querySelector('[data-new-chat-separator]')).not.toBeNull()

    // The + glyph is the row's LAST child (right edge); the provider icon leads.
    const newRow = container.querySelector<HTMLElement>('[data-new-chat="claude"]')!
    expect(newRow.firstElementChild?.querySelector('[data-p="claude"]')).not.toBeNull()
    expect(newRow.lastElementChild?.getAttribute('data-add-glyph')).toBe('true')
  })

  it('opens the FIRST ENABLED provider — a disabled leading provider is skipped', async () => {
    seed([CHAT_1])
    act(() =>
      state().setAgentProviders([
        { ...PROVIDERS[0], enabled: false },
        { ...PROVIDERS[1], enabled: true },
      ]),
    )
    const { container } = render(<Shell />)

    const newRow = container.querySelector<HTMLElement>('[data-new-chat]')!
    expect(newRow.getAttribute('data-new-chat')).toBe('codex')
    fireEvent.click(newRow)
    // '' is the workspace root, said explicitly — one shape for every surface.
    expect(createChatFn).toHaveBeenCalledWith('w1', 'codex', '')
    await waitFor(() => expect(agentBuffers()).toHaveLength(1))
  })

  /** Disable every provider. seed() resets them, so this runs AFTER it. */
  function disableAllProviders() {
    act(() => state().setAgentProviders(PROVIDERS.map((p) => ({ ...p, enabled: false }))))
  }

  it('renders no New rows when no provider is enabled', () => {
    seed([CHAT_1])
    disableAllProviders()
    const { container } = render(<Shell />)
    expect(container.querySelectorAll('[data-new-chat]')).toHaveLength(0)
    expect(container.querySelector('[data-new-chat-separator]')).toBeNull()
  })

  // …and says WHY, instead of leaving a dead surface. With every provider off
  // and no chats yet, this panel rendered COMPLETELY BLANK: no row, no message,
  // nothing to click and nothing to read.
  it('explains itself when no provider is enabled, and names where to fix it', () => {
    seed([])
    disableAllProviders()
    render(<Shell />)

    expect(screen.getByTestId('no-providers-notice')).toBeInTheDocument()
    expect(screen.getByText(/settings → providers/i)).toBeInTheDocument()
  })

  it('still explains itself when the workspace HAS chats but nothing can start one', () => {
    seed([CHAT_1])
    disableAllProviders()
    render(<Shell />)

    expect(screen.getByTestId('no-providers-notice')).toBeInTheDocument()
    expect(screen.getByText('First')).toBeInTheDocument() // the existing chats still list
  })

  it('shows no notice while a provider is enabled', () => {
    seed()
    render(<Shell />)
    expect(screen.queryByTestId('no-providers-notice')).toBeNull()
  })

  // Regression: NewChatRow is a real <button>, whose UA text-align is `center`.
  // Its label must carry text-left or it centers in the flex-1 span.
  it('the New-row label is left-aligned, not centered by the <button> default', () => {
    seed()
    render(<Shell />)
    expect(screen.getByText('New chat').className).toContain('text-left')
  })

  // Regression: `w-full` on the button = width:100% of the parent, but ROW_BASE
  // also has mx-1.5 (6px each side), so the row pokes 6px past the sidebar → a
  // stray horizontal scrollbar. The rows fill via the container being flex-col.
  it('the New rows fill width via flex-col stretch, never w-full (which overflows the sidebar)', () => {
    seed()
    const { container } = render(<Shell />)
    const btn = container.querySelector<HTMLElement>('[data-new-chat="claude"]')!
    expect(btn.className).not.toContain('w-full')
    expect(btn.parentElement?.className).toContain('flex-col')
  })

  it('empty state renders the single New chat row and no separator', () => {
    act(() => state().setAgentProviders(PROVIDERS))
    const { container } = render(<Shell />)
    expect(rowIds()).toEqual([])
    expect(container.querySelectorAll('[data-new-chat]')).toHaveLength(1)
    // No standing "New folder" affordance, here or anywhere: a folder is made
    // AROUND a selection from the right-click menu, which is the only way the
    // workspace tree makes one.
    expect(screen.queryByText('New folder')).toBeNull()
    expect(container.querySelector('[data-new-chat-separator]')).toBeNull()
  })

  it('renders no separator and no New rows when providers have not arrived', () => {
    seed([CHAT_1])
    act(() => state().setAgentProviders([]))
    const { container } = render(<Shell />)
    expect(container.querySelectorAll('[data-new-chat]')).toHaveLength(0)
    expect(container.querySelector('[data-new-chat-separator]')).toBeNull()
  })

  it('clicking the New chat row creates a chat for the first enabled provider and opens its pane', async () => {
    seed()
    render(<Shell />)
    fireEvent.click(screen.getByText('New chat'))

    expect(createChatFn).toHaveBeenCalledWith('w1', 'claude', '')
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
    render(<Shell />)
    fireEvent.click(screen.getByText('New chat'))

    await waitFor(() => expect(agentBuffers()).toHaveLength(1))
    expect(agentBuffers()[0]).toMatchObject({ chatId: 'c-new', name: 'Seeded title' })
  })

  it('a New row is a real <button> — keyboard-operable via Enter/Space, inert on other keys', async () => {
    seed()
    const user = userEvent.setup()
    const { container } = render(<Shell />)
    const newRow = container.querySelector<HTMLElement>('[data-new-chat="claude"]')!

    expect(newRow.tagName).toBe('BUTTON')
    newRow.focus()

    await user.keyboard('a')
    expect(createChatFn).not.toHaveBeenCalled()

    await user.keyboard('{Enter}')
    await user.keyboard(' ')
    expect(createChatFn).toHaveBeenCalledTimes(2)

    await waitFor(() => expect(agentBuffers()).toHaveLength(1))
  })

  it('a failed create opens no pane', async () => {
    const err = vi.spyOn(console, 'error').mockImplementation(() => {})
    createChatFn.mockRejectedValue(new Error('boom'))
    seed()
    render(<Shell />)
    fireEvent.click(screen.getByText('New chat'))

    await waitFor(() => expect(err).toHaveBeenCalled())
    expect(agentBuffers()).toHaveLength(0)
    expect(state().agentChats.activeChatId).toBeNull()
  })

  // The bug the user actually hit: the daemon could not find the claude binary
  // (launchd's PATH omits ~/.local/bin), every create 500'd, and the catch
  // dropped it into console.error — so the button did NOTHING, over and over.
  it('a failed create tells the user why, naming the CLI that is missing', async () => {
    const err = vi.spyOn(console, 'error').mockImplementation(() => {})
    createChatFn.mockRejectedValue(new ApiError('terminal: command not found: claude', 424))
    seed()
    render(<Shell />)
    fireEvent.click(screen.getByText('New chat'))

    await waitFor(() => expect(toastErrorFn).toHaveBeenCalledTimes(1))
    const [title, description] = toastErrorFn.mock.calls[0] as [string, string]
    expect(title).toContain('Claude')
    expect(title).toMatch(/isn.t installed/)
    expect(description).toMatch(/PATH/)
    err.mockRestore()
  })

  // ── Folders ───────────────────────────────────────────────────────

  it('a folder’s + starts a CHAT inside it, not another folder', async () => {
    seed([], [folder('f1', 'Spikes')])
    render(<Shell />)
    await act(async () => {})

    fireEvent.click(screen.getByRole('button', { name: 'New chat in Spikes' }))

    // "+" on a row means "add a child of the tree's primary kind", and the
    // primary kind here is a conversation.
    expect(createChatFn).toHaveBeenCalledWith('w1', 'claude', 'f1')
    expect(createChatFolderFn).not.toHaveBeenCalled()
    // ONE call: the parentage rides on the create, so there is no second write
    // to fail and strand the new chat at the root.
    await act(async () => {})
    expect(setChatPlacementFn).not.toHaveBeenCalled()
  })

  it('a folded folder’s + opens it before the new chat lands inside', async () => {
    seed(
      [chat('cf', 'Filed', 'claude', '2026-01-05T00:00:00Z', { parentId: 'f1' })],
      [folder('f1', 'Spikes')],
    )
    render(<Shell />)
    await act(async () => {})
    fireEvent.click(screen.getAllByRole('button', { name: /collapse/i })[0])
    expect(rowIds()).toEqual(['f1'])

    fireEvent.click(screen.getByRole('button', { name: 'New chat in Spikes' }))

    // Putting something into a row you cannot see is not what "+ in here"
    // means — the row opens, and what it already held comes back with it.
    expect(rowIds()).toEqual(['f1', 'cf'])
    await waitFor(() => expect(createChatFn).toHaveBeenCalledWith('w1', 'claude', 'f1'))
  })

  it('a folder’s + does nothing when no provider can start a chat', async () => {
    seed([], [folder('f1', 'Spikes')])
    disableAllProviders()
    render(<Shell />)
    await act(async () => {})

    // The New-chat row is not drawn in this state, but a folder's "+" is.
    fireEvent.click(screen.getByRole('button', { name: 'New chat in Spikes' }))

    expect(createChatFn).not.toHaveBeenCalled()
  })

  it('⌘-clicking a folder collects it without folding it', async () => {
    seed(
      [chat('cf', 'Filed', 'claude', '2026-01-05T00:00:00Z', { parentId: 'f1' })],
      [folder('f1', 'Spikes')],
    )
    render(<Shell />)
    await act(async () => {})

    fireEvent.click(rowFor('f1'), { metaKey: true })

    expect(rowFor('f1').getAttribute('aria-selected')).toBe('true')
    // The selection gesture must not also fold the row it is collecting.
    expect(rowIds()).toEqual(['f1', 'cf'])

    fireEvent.click(rowFor('f1'), { metaKey: true })
    expect(rowFor('f1').getAttribute('aria-selected')).toBe('false')
  })

  // ── Threading ─────────────────────────────────────────────────────
  //
  // The whole point of this panel: a chat can hold other chats, and a child is a
  // THREAD that reads its parent's turns. There are two ways to make one and they
  // are one code path — the row's "+" and the menu's "New thread" both mean
  // "start a chat under this one".

  it('a chat row’s + starts a thread of that chat and opens it', async () => {
    seed()
    render(<Shell />)

    fireEvent.click(screen.getByRole('button', { name: 'New thread in First' }))

    // The parentage rides on the CREATE. The daemon mints the chat, writes the
    // edge and only then starts the runner, so the thread has its lineage before
    // its first CLI exists — create-then-place ran the very turn the user asked
    // the thread for with the agent as a stranger.
    expect(createChatFn).toHaveBeenCalledWith('w1', 'claude', 'c1')
    await act(async () => {})
    expect(setChatPlacementFn).not.toHaveBeenCalled()
    // …and it opens, because a thread you have just made is the one you are
    // about to talk to.
    expect(state().agentChats.activeChatId).toBe('c-new')
    expect(agentBuffers().map((b) => b.chatId)).toContain('c-new')
  })

  it('the new thread lands as a child row of the chat it was made from', async () => {
    seed()
    render(<Shell />)

    fireEvent.click(screen.getByRole('button', { name: 'New thread in First' }))
    // The `created` frame arrives on the WS feed after the POST resolves.
    await act(async () => {})
    act(() =>
      state().upsertAgentChat(
        chat('c-new', 'Thread', 'claude', '2026-02-01T00:00:00Z', { parentId: 'c1' }),
      ),
    )

    expect(rowIds()).toEqual(['c2', 'c1', 'c-new'])
    expect(depthOf('c-new')).toBe(1)
  })

  it('the menu’s "New thread" is the same gesture', async () => {
    seed()
    render(<Shell />)

    fireEvent.contextMenu(rowFor('c1'))
    fireEvent.click(screen.getByText('New thread'))

    expect(createChatFn).toHaveBeenCalledWith('w1', 'claude', 'c1')
    await act(async () => {})
    expect(setChatPlacementFn).not.toHaveBeenCalled()
  })

  it('a FOLDED chat opens before its new thread lands inside it', async () => {
    seed(THREADED)
    render(<Shell />)
    fireEvent.click(screen.getAllByRole('button', { name: /collapse/i })[0])
    expect(rowIds()).toEqual(['p1'])

    fireEvent.click(screen.getByRole('button', { name: 'New thread in Parent' }))

    // Threading into a chat you cannot see is not what "+" means here either.
    expect(rowIds()).toEqual(['p1', 't1', 't2', 't2a'])
    await waitFor(() => expect(createChatFn).toHaveBeenCalledWith('w1', 'claude', 'p1'))
  })

  it('a chat row’s + does nothing when no provider can start a chat', () => {
    seed()
    disableAllProviders()
    render(<Shell />)

    fireEvent.click(screen.getByRole('button', { name: 'New thread in First' }))

    expect(createChatFn).not.toHaveBeenCalled()
  })

  it('a failed thread create says so rather than failing silently', async () => {
    createChatFn.mockRejectedValue(new Error('claude is not installed'))
    seed()
    render(<Shell />)

    fireEvent.click(screen.getByRole('button', { name: 'New thread in First' }))

    await waitFor(() => expect(toastErrorFn).toHaveBeenCalled())
    expect(setChatPlacementFn).not.toHaveBeenCalled()
  })

  // ── The right-click menu ──────────────────────────────────────────
  //
  // A folder is made AROUND a selection, which is the only way the workspace
  // tree makes one. What the menu acts on comes from `dragSubjectsFor()` — the
  // same function the drag uses — so a right-click and a drag can never disagree
  // about which rows they are holding.

  const menuItems = () =>
    Array.from(document.querySelectorAll('[role="menuitem"]')).map((el) => el.textContent)

  it('takes the clicked row alone when it is outside the selection', () => {
    seed(THREE)
    render(<Shell />)
    fireEvent.click(rowFor('c1'), { metaKey: true })

    fireEvent.contextMenu(rowFor('c3'))

    expect(menuItems()).toEqual(['New thread', 'Group into a folder', 'Delete chat'])
    // Pressing an unselected row is not a way to extend a selection.
    expect(rowFor('c1').getAttribute('aria-selected')).toBe('true')
    expect(rowFor('c3').getAttribute('aria-selected')).toBe('false')
  })

  it('takes the whole multiselection when the clicked row is part of it', () => {
    seed(THREE)
    render(<Shell />)
    fireEvent.click(rowFor('c1'), { metaKey: true })
    fireEvent.click(rowFor('c2'), { metaKey: true })

    fireEvent.contextMenu(rowFor('c2'))

    expect(menuItems()).toEqual(['Group 2 into a folder', 'Delete 2 chats'])
  })

  it('names a folder’s deletion for what it takes', async () => {
    seed([], [folder('f1', 'Spikes')])
    render(<Shell />)
    await act(async () => {})

    fireEvent.contextMenu(rowFor('f1'))

    expect(menuItems()).toEqual(['Group into a folder', 'Delete folder'])
  })

  it('opens no menu on a row this tree does not own', () => {
    seed()
    render(<Shell />)

    fireEvent.contextMenu(screen.getByText('New chat'))

    // The New-chat row is not a tree row, and neither is the sidebar next door:
    // a menu about nothing is worse than the row simply not having one.
    expect(menuItems()).toEqual([])
  })

  it('groups the selection into a folder where those rows already live', async () => {
    createChatFolderFn.mockResolvedValue({
      folder: folder('f-new', 'New folder', { parentId: 'p1' }),
      shifted: [],
    })
    seed([
      chat('p1', 'Parent', 'claude', '2026-01-01T00:00:00Z', { order: 0 }),
      chat('t1', 'Thread one', 'claude', '2026-01-02T00:00:00Z', { parentId: 'p1', order: 0 }),
      chat('t2', 'Thread two', 'codex', '2026-01-03T00:00:00Z', { parentId: 'p1', order: 1 }),
    ])
    render(<Shell />)
    fireEvent.click(rowFor('t1'), { metaKey: true })
    fireEvent.click(rowFor('t2'), { metaKey: true })
    fireEvent.contextMenu(rowFor('t2'))

    await act(async () => {
      fireEvent.click(screen.getByText('Group 2 into a folder'))
    })

    // A SIBLING of the rows it collects — the gesture is "these belong
    // together", not "start a folder at the root and drag them back to it".
    expect(createChatFolderFn).toHaveBeenCalledWith('w1', 'New folder', 'p1')
    // Filed in order: each `order` indexes the folder as it stands once the
    // previous row has landed.
    await waitFor(() => expect(setChatPlacementFn).toHaveBeenCalledTimes(2))
    expect(setChatPlacementFn.mock.calls.map((c) => [c[1], c[2]])).toEqual([
      ['t1', { parentId: 'f-new', order: 0 }],
      ['t2', { parentId: 'f-new', order: 1 }],
    ])
    // Named for you, with the caret already in it and its contents around it.
    expect(screen.getByDisplayValue('New folder')).toBeTruthy()
  })

  it('opens no rename editor when the daemon refuses the folder', async () => {
    createChatFolderFn.mockRejectedValue(new Error('nope'))
    seed(THREE)
    render(<Shell />)
    fireEvent.contextMenu(rowFor('c1'))

    await act(async () => {
      fireEvent.click(screen.getByText('Group into a folder'))
    })

    // A caret blinking in a row that does not exist is worse than no row at all.
    await waitFor(() => expect(toastErrorFn).toHaveBeenCalled())
    // By placeholder, not by role: the panel's search field is a textbox too.
    expect(screen.queryByPlaceholderText('folder-name')).toBeNull()
    expect(setChatPlacementFn).not.toHaveBeenCalled()
  })

  it('puts the rows back when the daemon refuses to file them', async () => {
    createChatFolderFn.mockResolvedValue({ folder: folder('f-new', 'New folder'), shifted: [] })
    setChatPlacementFn.mockRejectedValue(new Error('nope'))
    seed(THREE)
    render(<Shell />)
    fireEvent.contextMenu(rowFor('c1'))

    await act(async () => {
      fireEvent.click(screen.getByText('Group into a folder'))
    })

    // The folder stays — it exists on the server now, and deleting it would be a
    // second write racing the one that just failed — but the row goes home.
    await waitFor(() => expect(toastErrorFn).toHaveBeenCalled())
    expect(depthOf('c1')).toBe(0)
  })

  it('renaming a folder writes the name, not a chat rename', () => {
    seed([], [folder('f1', 'Spikes')])
    render(<Shell />)

    fireEvent.doubleClick(screen.getByText('Spikes'))
    const input = screen.getByDisplayValue('Spikes')
    fireEvent.change(input, { target: { value: 'Experiments' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    expect(updateChatFolderFn).toHaveBeenCalledWith('w1', 'f1', { name: 'Experiments' })
    expect(renameChatFn).not.toHaveBeenCalled()
    expect(screen.getByText('Experiments')).toBeTruthy()
  })

  // ── Rename ────────────────────────────────────────────────────────

  it('double-click renames: optimistic title + renameChat call', () => {
    seed()
    render(<Shell />)
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
    render(<Shell />)
    fireEvent.doubleClick(screen.getByText('First'))
    fireEvent.keyDown(screen.getByDisplayValue('First'), { key: 'Enter' })

    expect(renameChatFn).not.toHaveBeenCalled()
    expect(screen.getByText('First')).toBeTruthy()
  })

  it('a failed rename is logged (the WS title_set frame remains the source of truth)', async () => {
    const err = vi.spyOn(console, 'error').mockImplementation(() => {})
    renameChatFn.mockRejectedValue(new Error('nope'))
    seed()
    render(<Shell />)

    fireEvent.doubleClick(screen.getByText('First'))
    const input = screen.getByDisplayValue('First')
    fireEvent.change(input, { target: { value: 'Renamed' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    await waitFor(() => expect(err).toHaveBeenCalled())
  })

  it('cancelling a rename leaves the title untouched', () => {
    seed()
    render(<Shell />)
    fireEvent.doubleClick(screen.getByText('First'))
    fireEvent.keyDown(screen.getByDisplayValue('First'), { key: 'Escape' })

    expect(renameChatFn).not.toHaveBeenCalled()
    expect(screen.getByText('First')).toBeTruthy()
  })

  // ── Drag ──────────────────────────────────────────────────────────

  it('dropping a chat INTO another makes it a thread of it', async () => {
    seed(THREE)
    render(<Shell />)

    dragOnto('c3', 'c1', 'middle')

    // Optimistic first: the indent is the statement, and it lands before the
    // round trip rather than after it.
    expect(depthOf('c3')).toBe(1)
    await waitFor(() => expect(placements()).toEqual([['c3', 'c1']]))
  })

  it('dragging a thread out to the root makes it standalone again', async () => {
    seed([
      chat('p1', 'Parent', 'claude', '2026-01-01T00:00:00Z', { order: 0 }),
      chat('t1', 'Thread', 'claude', '2026-01-02T00:00:00Z', { parentId: 'p1', order: 0 }),
      chat('c9', 'Other', 'codex', '2026-01-03T00:00:00Z', { order: 1 }),
    ])
    render(<Shell />)
    expect(depthOf('t1')).toBe(1)

    dragOnto('t1', 'c9', 'bottom')

    expect(depthOf('t1')).toBe(0)
    await waitFor(() => expect(placements()).toEqual([['t1', '']]))
  })

  it('dropping on a row’s lower edge lands AFTER it, in its own level', async () => {
    seed(THREE)
    render(<Shell />)
    expect(rowIds()).toEqual(['c1', 'c2', 'c3'])

    dragOnto('c3', 'c1', 'bottom')

    expect(rowIds()).toEqual(['c1', 'c3', 'c2'])
    await waitFor(() =>
      expect(setChatPlacementFn).toHaveBeenCalledWith('w1', 'c3', {
        parentId: '',
        order: 1,
      }),
    )
  })

  it('dropping on a row’s upper edge lands BEFORE it', async () => {
    seed(THREE)
    render(<Shell />)

    dragOnto('c3', 'c1', 'top')

    expect(rowIds()).toEqual(['c3', 'c1', 'c2'])
    await waitFor(() =>
      expect(setChatPlacementFn).toHaveBeenCalledWith('w1', 'c3', {
        parentId: '',
        order: 0,
      }),
    )
  })

  it('the gap under an EXPANDED parent is its first child’s slot, not the slot after its subtree', async () => {
    seed([
      chat('p1', 'Parent', 'claude', '2026-01-01T00:00:00Z', { order: 0 }),
      chat('t1', 'Thread', 'claude', '2026-01-02T00:00:00Z', { parentId: 'p1', order: 0 }),
      chat('c9', 'Other', 'codex', '2026-01-03T00:00:00Z', { order: 1 }),
    ])
    render(<Shell />)

    dragOnto('c9', 'p1', 'bottom')

    // That gap is where the indicator was drawn, so that is where the row lands:
    // "after" an open parent is a re-parent.
    expect(depthOf('c9')).toBe(1)
    await waitFor(() => expect(placements()).toEqual([['c9', 'p1']]))
  })

  it('refuses a row into its own descendant, and draws no mark for it', () => {
    seed([
      chat('p1', 'Parent', 'claude', '2026-01-01T00:00:00Z'),
      chat('t1', 'Thread', 'claude', '2026-01-02T00:00:00Z', { parentId: 'p1' }),
    ])
    render(<Shell />)

    dragOver('p1', 't1', 'middle')

    // Detaching a subtree from the tree is the classic way to lose rows. A
    // refusal draws NOTHING — no line, no fill.
    expect(dropLine()?.style.display).toBe('none')
    expect(rowFor('t1').className).not.toContain('bg-sidebar-drop-nest')

    pointerUp(10, aimAt('t1', 'middle'))
    expect(setChatPlacementFn).not.toHaveBeenCalled()
  })

  it('refuses a row onto itself', () => {
    seed(THREE)
    render(<Shell />)

    dragOnto('c1', 'c1', 'middle')

    expect(setChatPlacementFn).not.toHaveBeenCalled()
  })

  it('sends nothing when the drop would change nothing', () => {
    // Dropping a row onto its own edge is the commonest accident in a list, and a
    // write per accident is a write the daemon has to serialise behind the ones
    // that mean something.
    seed(THREE)
    render(<Shell />)

    dragOnto('c2', 'c1', 'bottom')

    expect(rowIds()).toEqual(['c1', 'c2', 'c3'])
    expect(setChatPlacementFn).not.toHaveBeenCalled()
  })

  it('a hairline says BETWEEN and a fill says INSIDE — never both', () => {
    seed(THREE)
    render(<Shell />)

    dragOver('c3', 'c1', 'top')
    expect(dropLine()?.style.display).toBe('block')
    expect(rowFor('c1').className).not.toContain('bg-sidebar-drop-nest')

    pointerMove(10, aimAt('c1', 'middle'))
    expect(dropLine()?.style.display).toBe('none')
    expect(rowFor('c1').className).toContain('bg-sidebar-drop-nest')

    pointerUp(10, aimAt('c1', 'middle'))
    expect(dropLine()).toBeNull()
  })

  it('fades the source row and carries a clone of it', () => {
    seed(THREE)
    render(<Shell />)

    dragOver('c3', 'c1', 'top')

    // What is in the AIR is a clone, promoted to the active treatment (lifted is
    // what active already draws); the source stays where it is and fades.
    expect(rowFor('c3').className).toContain('opacity-40')
    expect(ghost()?.textContent).toContain('Third')

    pointerUp(10, aimAt('c1', 'top'))
    expect(ghost()).toBeNull()
    expect(rowFor('c3').className).not.toContain('opacity-40')
  })

  it('moves the ghost with a compositor-only transform, never left/top', () => {
    seed(THREE)
    render(<Shell />)
    dragOver('c3', 'c1', 'top')

    pointerMove(120, 300)

    // Measured on the live app: one pointermove per frame cost 60ms with
    // left/top against 8ms with a translate, because left/top repaints whatever
    // the ghost is over — usually the editor pane.
    expect(ghost()?.style.transform).toContain('translate3d')
    expect(ghost()?.style.left).toBe('')
    expect(ghost()?.style.top).toBe('')
    pointerUp(120, 300)
  })

  it('⌘-click collects rows, and one drag carries them all in tree order', async () => {
    seed(THREE)
    render(<Shell />)

    fireEvent.click(rowFor('c1'), { metaKey: true })
    fireEvent.click(rowFor('c2'), { metaKey: true })
    // A selection gesture is not a request to open anything.
    expect(agentBuffers()).toHaveLength(0)
    expect(rowFor('c1').getAttribute('aria-selected')).toBe('true')

    dragOver('c1', 'c3', 'bottom')
    expect(
      document.querySelector('[data-drag-ghost-rows]')?.getAttribute('data-drag-ghost-rows'),
    ).toBe('2')
    pointerUp(10, aimAt('c3', 'bottom'))

    // Both rows moved, in the order they are drawn, and each `order` indexes the
    // level as it stands once the previous call has landed.
    await waitFor(() => expect(setChatPlacementFn).toHaveBeenCalledTimes(2))
    expect(setChatPlacementFn.mock.calls.map((c) => [c[1], c[2]])).toEqual([
      ['c1', { parentId: '', order: 1 }],
      ['c2', { parentId: '', order: 2 }],
    ])
  })

  it('⌘-clicking a collected row again drops it from the selection', () => {
    seed(THREE)
    render(<Shell />)

    fireEvent.click(rowFor('c1'), { metaKey: true })
    fireEvent.click(rowFor('c1'), { metaKey: true })
    expect(rowFor('c1').getAttribute('aria-selected')).toBe('false')
  })

  it('grabbing a row OUTSIDE the selection moves that row alone', async () => {
    seed(THREE)
    render(<Shell />)
    fireEvent.click(rowFor('c1'), { metaKey: true })

    dragOnto('c3', 'c1', 'top')

    // Pressing an unselected row is not a way to extend a selection.
    await waitFor(() => expect(setChatPlacementFn).toHaveBeenCalledTimes(1))
    expect(setChatPlacementFn.mock.calls[0][1]).toBe('c3')
  })

  it('springs a folded row open when the drag rests inside it', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    try {
      seed([
        chat('p1', 'Parent', 'claude', '2026-01-01T00:00:00Z', { order: 0 }),
        chat('t1', 'Thread', 'claude', '2026-01-02T00:00:00Z', { parentId: 'p1', order: 0 }),
        chat('c9', 'Other', 'codex', '2026-01-03T00:00:00Z', { order: 1 }),
      ])
      render(<Shell />)
      fireEvent.click(screen.getAllByRole('button', { name: /collapse/i })[0])
      expect(rowIds()).toEqual(['p1', 'c9'])

      dragOver('c9', 'p1', 'middle')
      act(() => vi.advanceTimersByTime(CHAT_SPRING_OPEN_MS + 10))

      // Resting inside a folded row is asking to go in, so it opens — and the row
      // you were aiming at is now reachable without letting go.
      expect(rowIds()).toEqual(['p1', 't1', 'c9'])
      pointerUp(10, aimAt('p1', 'middle'))
    } finally {
      // Drain what the drop scheduled BEFORE handing the clock back: the release
      // arms a one-shot click trap that a setTimeout(0) is supposed to remove,
      // and a fake timer discarded on the way out leaves that trap armed —
      // swallowing the first click of whatever test runs next.
      act(() => vi.runOnlyPendingTimers())
      vi.useRealTimers()
    }
  })

  it('does not spring a row open from its EDGE — that is asking to land beside it', () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    try {
      seed([
        chat('p1', 'Parent', 'claude', '2026-01-01T00:00:00Z', { order: 0 }),
        chat('t1', 'Thread', 'claude', '2026-01-02T00:00:00Z', { parentId: 'p1', order: 0 }),
        chat('c9', 'Other', 'codex', '2026-01-03T00:00:00Z', { order: 1 }),
      ])
      render(<Shell />)
      fireEvent.click(screen.getAllByRole('button', { name: /collapse/i })[0])

      dragOver('c9', 'p1', 'top')
      act(() => vi.advanceTimersByTime(CHAT_SPRING_OPEN_MS + 10))

      expect(rowIds()).toEqual(['p1', 'c9'])
      pointerUp(10, aimAt('p1', 'top'))
    } finally {
      // Drain what the drop scheduled BEFORE handing the clock back: the release
      // arms a one-shot click trap that a setTimeout(0) is supposed to remove,
      // and a fake timer discarded on the way out leaves that trap armed —
      // swallowing the first click of whatever test runs next.
      act(() => vi.runOnlyPendingTimers())
      vi.useRealTimers()
    }
  })

  it('leaving a row before it springs cancels the spring', () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    try {
      seed([
        chat('p1', 'Parent', 'claude', '2026-01-01T00:00:00Z', { order: 0 }),
        chat('t1', 'Thread', 'claude', '2026-01-02T00:00:00Z', { parentId: 'p1', order: 0 }),
        chat('c9', 'Other', 'codex', '2026-01-03T00:00:00Z', { order: 1 }),
      ])
      render(<Shell />)
      fireEvent.click(screen.getAllByRole('button', { name: /collapse/i })[0])

      dragOver('c9', 'p1', 'middle')
      // Held still inside the same row: the armed spring is KEPT, not restarted,
      // or a hand that shakes would hold a folded row shut indefinitely.
      pointerMove(10, aimAt('p1', 'middle') + 1)
      // …and then the pointer moves on before it fires.
      pointerMove(10, aimAt('p1', 'top'))
      act(() => vi.advanceTimersByTime(CHAT_SPRING_OPEN_MS + 10))

      // Crossing a folded row on the way somewhere else must not disturb it.
      expect(rowIds()).toEqual(['p1', 'c9'])
      pointerUp(10, aimAt('p1', 'top'))
    } finally {
      act(() => vi.runOnlyPendingTimers())
      vi.useRealTimers()
    }
  })

  it('stops the edge scroller when the pointer leaves the panel sideways', () => {
    seed(THREE)
    render(<Shell />)
    layout()

    pointerDown(rowFor('c1'), 0, 10, 10)
    pointerMove(10, 20)
    // The band is a function of Y alone, so a row carried sideways to the bin —
    // which asks the hand to hold still — would otherwise keep the list running
    // out from under the drag at 14px a frame.
    pointerMove(900, 20)

    expect(ghost()).not.toBeNull()
    pointerUp(900, 20)
    expect(setChatPlacementFn).not.toHaveBeenCalled()
  })

  it('a drag that never crosses the threshold does not move anything (and the click still selects)', () => {
    seed()
    render(<Shell />)
    layout()

    pointerDown(rowFor('c2'), 0, 10, 10)
    pointerMove(11, 11) // below the 5px threshold
    pointerUp(11, 11)
    fireEvent.click(rowFor('c2'))

    expect(setChatPlacementFn).not.toHaveBeenCalled()
    expect(state().agentChats.activeChatId).toBe('c2')
  })

  it('the click that ends a real drag never opens the dragged row', () => {
    seed(THREE)
    render(<Shell />)

    dragOnto('c3', 'c1', 'top')
    fireEvent.click(rowFor('c3')) // the browser-generated post-drop click

    expect(state().agentChats.activeChatId).toBeNull()
    expect(agentBuffers()).toHaveLength(0)
  })

  it('a release over nothing moves nothing', () => {
    seed(THREE)
    render(<Shell />)
    layout()

    pointerDown(rowFor('c3'), 0, 10, 90)
    pointerMove(10, 100)
    pointerMove(10, 900) // past the last row, short of the bin
    pointerUp(10, 900)

    expect(setChatPlacementFn).not.toHaveBeenCalled()
    expect(deleteChatFn).not.toHaveBeenCalled()
  })

  it('a non-primary button never starts a drag', () => {
    seed()
    render(<Shell />)
    layout()

    pointerDown(rowFor('c2'), 2)
    pointerMove(10, 100)
    pointerUp(10, 100)

    expect(setChatPlacementFn).not.toHaveBeenCalled()
    expect(ghost()).toBeNull()
  })

  it('pointercancel aborts an in-flight drag', () => {
    seed(THREE)
    render(<Shell />)
    layout()

    pointerDown(rowFor('c2'), 0, 10, 50)
    pointerMove(10, 60)
    act(() => window.dispatchEvent(new MouseEvent('pointercancel', { bubbles: true })))

    expect(ghost()).toBeNull()
    expect(veilUp()).toBe(false)

    pointerUp(10, 60)
    expect(setChatPlacementFn).not.toHaveBeenCalled()
  })

  it('a second pointer mid-drag is ignored', () => {
    seed(THREE)
    render(<Shell />)
    layout()

    pointerDown(rowFor('c3'), 0, 10, 90)
    pointerMove(10, 100)
    pointerDown(rowFor('c1'), 0, 10, 10) // a second press while one is in flight
    pointerUp(10, aimAt('c1', 'top'))

    // The drag that was already running is the one that commits.
    expect(setChatPlacementFn.mock.calls.map((c) => c[1])).toEqual(['c3'])
  })

  it('snaps the level back and says why when the daemon refuses a move', async () => {
    setChatPlacementFn.mockRejectedValue(new Error('a chat cannot be filed under itself'))
    seed(THREE)
    render(<Shell />)

    dragOnto('c3', 'c1', 'top')
    expect(rowIds()).toEqual(['c3', 'c1', 'c2'])

    // A row that visibly moves and returns with no explanation reads as the drag
    // having missed, so the user repeats the gesture and gets the same nothing.
    await waitFor(() => expect(rowIds()).toEqual(['c1', 'c2', 'c3']))
    expect(toastErrorFn).toHaveBeenCalledWith('a chat cannot be filed under itself')
  })
})

/**
 * Removal: the editor pane, and the tray it drops into.
 *
 * The whole gesture is the SIDEBAR'S — carry the row onto the pane, wait out the
 * arming dwell, release, and the row waits eight seconds in the tray where you
 * can still keep it. There is no bin in this panel's footer any more, and no
 * confirm dialog either; the undo window is what makes it safe.
 *
 * Fake timers, and deliberately NOT `shouldAdvanceTime`: both the dwell and the
 * drain are measured in milliseconds, and a fake clock that also creeps with the
 * real one turns "one tick short" into a race with however long the test took to
 * get there. Nothing in here uses `userEvent`, which is the only thing in this
 * file that needs a moving clock.
 */
describe('AgentChatsPanel removal', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    // Every release arms a one-shot capture-phase click trap that the browser
    // drops on the next macrotask. Under fake timers nothing drops it, and it
    // would silently swallow the first click of the NEXT test.
    act(() => {
      vi.advanceTimersByTime(1)
    })
    vi.useRealTimers()
  })

  /** Run the tray's clock out and let the deletes it fires settle. */
  async function drainTray() {
    await act(async () => {
      vi.advanceTimersByTime(REMOVAL_DRAIN_MS)
    })
    await act(async () => {})
  }

  // ── The pane's veil ───────────────────────────────────────────────

  it('veils the editor pane for the WHOLE drag, and arms it only after the dwell', () => {
    seed()
    render(<Shell />)
    expect(veilUp()).toBe(false)

    layout()
    const from = rowFor('c1').getBoundingClientRect()
    pointerDown(rowFor('c1'), 0, 10, from.top + 20)
    pointerMove(10, from.top + 30)

    // Up from the first frame, naming what would go — the zone used to be
    // invisible until it armed, which made the one gesture that deletes anything
    // discoverable only to someone who already knew it was there.
    expect(veilUp()).toBe(true)
    expect(veilArmed()).toBe(false)
    expect(veilTitle()).toBe('Drop here to remove First')

    pointerMove(PANE_X, 100)
    act(() => vi.advanceTimersByTime(PANE_ARM_MS))
    expect(veilArmed()).toBe(true)
    expect(veilTitle()).toBe('Release to remove First')

    pointerUp(PANE_X, 100)
    expect(veilUp()).toBe(false)
  })

  it('the veil says a chat takes its threads with it', () => {
    seed(THREADED)
    render(<Shell />)

    dragOverPane('p1')

    // p1 holds t1, t2 and t2a. The cascade is the thing a user cannot see from
    // the pane, so the veil is where it has to be said.
    expect(veilTitle()).toBe('Release to remove Parent')
    expect(veilDetail()).toBe('3 nested rows go with it · You will have 8 seconds to undo')
    pointerUp(PANE_X, 100)
  })

  it('the veil says a FOLDER does not take its chats', async () => {
    seed(
      [chat('cf', 'Filed', 'claude', '2026-01-05T00:00:00Z', { parentId: 'f1' })],
      [folder('f1', 'Spikes')],
    )
    render(<Shell />)
    await act(async () => {})

    dragOverPane('f1')

    // Deliberately the OTHER rule: a folder is a way of looking at chats and the
    // chats outlive it.
    expect(veilTitle()).toBe('Release to remove Spikes')
    expect(veilDetail()).toBe('Its contents move up one level · You will have 8 seconds to undo')
    pointerUp(PANE_X, 100)
  })

  it('the veil counts a multiselection rather than naming one of it', () => {
    seed(THREE)
    render(<Shell />)
    fireEvent.click(rowFor('c1'), { metaKey: true })
    fireEvent.click(rowFor('c2'), { metaKey: true })

    dragOverPane('c1')

    expect(veilTitle()).toBe('Release to remove 2 rows')
    pointerUp(PANE_X, 100)
  })

  it('names an untitled chat the same way the row does', () => {
    // The daemon titles a chat once it has something to title it FROM, so a
    // fresh one is blank — and the veil and the tray row both read "Release to
    // remove " with nothing after it until this is normalised.
    seed([chat('c1', '', 'claude', '2026-01-01T00:00:00Z')])
    render(<Shell />)

    dragOverPane('c1')

    expect(veilTitle()).toBe('Release to remove Untitled chat')
    pointerUp(PANE_X, 100)
  })

  it('a lone chat with no threads promises only the undo window', () => {
    seed()
    render(<Shell />)
    dragOverPane('c1')
    expect(veilDetail()).toBe('You will have 8 seconds to undo')
    pointerUp(PANE_X, 100)
  })

  it('never veils, and never arms, for a row that left the store mid-drag', () => {
    seed()
    render(<Shell />)
    layout()
    const from = rowFor('c1').getBoundingClientRect()
    pointerDown(rowFor('c1'), 0, 10, from.top + 20)
    pointerMove(10, from.top + 30)
    expect(veilUp()).toBe(true)

    // A `deleted` frame lands while the row is in the air. Better to offer no
    // target than one that refuses on release.
    act(() => state().removeAgentChat('c1'))
    pointerMove(PANE_X, 100)
    act(() => vi.advanceTimersByTime(PANE_ARM_MS))

    expect(veilUp()).toBe(false)
    pointerUp(PANE_X, 100)
    expect(trayIds()).toEqual([])
  })

  // ── The drop ──────────────────────────────────────────────────────

  it('a release over an ARMED pane holds the row in the tray, hidden but not deleted', () => {
    seed()
    render(<Shell />)

    dragToPane('c1')

    expect(screen.queryByRole('alertdialog')).toBeNull()
    expect(rowIds()).toEqual(['c2'])
    expect(trayIds()).toEqual(['c1'])
    expect(trayRows()[0]).toContain('First')
    // Nothing has been sent, and the chat is still in the store — a hold is not
    // a delete, which is what makes Keep a matter of dropping an id.
    expect(deleteChatFn).not.toHaveBeenCalled()
    expect(state().agentChats.chats.map((c) => c.id)).toEqual(['c1', 'c2'])
  })

  it('the held row wears the provider mark its sidebar row wore', () => {
    seed()
    render(<Shell />)
    // c1 is a claude chat, c2 a codex one — the fixture gives each provider its
    // own artwork, so the glyph identifies the row.
    const rowGlyph = rowFor('c1').querySelector('[data-p="claude"]')
    expect(rowGlyph).not.toBeNull()

    dragToPane('c1')

    const trayRow = document.querySelector('[data-removal-entry]')!
    expect(trayRow.querySelector('[data-p="claude"]')).not.toBeNull()
    // Static: a chat on its way out is not doing a turn.
    expect(trayRow.querySelector('[data-flicker-spinner]')).toBeNull()
  })

  it('a release before the dwell has elapsed removes nothing', () => {
    seed()
    render(<Shell />)

    dragToPane('c1', PANE_ARM_MS - 1)

    // A long reorder crosses this pane on its way; a pane that removed on
    // release would make that transit a loaded gun.
    expect(trayIds()).toEqual([])
    expect(rowIds()).toEqual(['c2', 'c1'])
  })

  it('the delete fires only once the tray’s clock runs out', async () => {
    seed()
    render(<Shell />)
    fireEvent.click(rowFor('c1')) // open its pane tab first
    expect(agentBuffers()).toHaveLength(1)

    dragToPane('c1')
    // The tab is untouched while the row is merely held: undo has to put back
    // exactly what was there.
    expect(agentBuffers()).toHaveLength(1)

    await drainTray()

    expect(deleteChatFn).toHaveBeenCalledWith('w1', 'c1', undefined)
    expect(trayIds()).toEqual([])
    expect(agentBuffers()).toHaveLength(0)
    expect(state().agentChats.activeChatId).toBeNull()
  })

  it('Keep puts the row back with nothing sent', () => {
    seed()
    render(<Shell />)

    dragToPane('c1')
    fireEvent.click(screen.getByRole('button', { name: /keep first/i }))

    expect(rowIds()).toEqual(['c2', 'c1'])
    expect(trayIds()).toEqual([])
    expect(deleteChatFn).not.toHaveBeenCalled()
  })

  it('a held chat takes its threads off screen with it, and gives them all back', () => {
    seed(THREADED)
    render(<Shell />)

    dragToPane('p1')
    expect(rowIds()).toEqual([])
    // The count beside the tray row is what says how much else goes.
    expect(useRemovalTrayStore.getState().entries[0].extra).toBe(3)

    fireEvent.click(screen.getByRole('button', { name: /keep parent/i }))
    expect(rowIds()).toEqual(['p1', 't1', 't2', 't2a'])
  })

  it('deletes a chat subtree deepest first', async () => {
    seed(THREADED)
    render(<Shell />)

    dragToPane('p1')
    await drainTray()

    // A parent deleted before its children leaves them pointing at nothing for
    // as long as the requests are in flight.
    expect(deleteChatFn.mock.calls.map((c) => c[1])).toEqual(['t1', 't2a', 't2', 'p1'])
  })

  it('a held FOLDER shows its chats promoted, and files them back on Keep', async () => {
    seed(
      [chat('cf', 'Filed', 'claude', '2026-01-05T00:00:00Z', { parentId: 'f1' })],
      [folder('f1', 'Spikes')],
    )
    render(<Shell />)
    // Let the panel's one-shot folder GET land first: its answer is the folder
    // list as it was, and applied after a hold it would put the row back.
    await act(async () => {})
    expect(depthOf('cf')).toBe(1)

    dragToPane('f1')

    // The preview has to show what the commit will do: the chat moves up to
    // where the folder sat, rather than the folder simply vanishing from around
    // it (which would re-root the chat — a different place).
    expect(rowIds()).toEqual(['cf'])
    expect(depthOf('cf')).toBe(0)

    fireEvent.click(screen.getByRole('button', { name: /keep spikes/i }))
    expect(depthOf('cf')).toBe(1)
  })

  it('deleting a folder promotes its chats and never deletes one', async () => {
    seed(
      [chat('cf', 'Filed', 'claude', '2026-01-05T00:00:00Z', { parentId: 'f1' })],
      [folder('f1', 'Spikes')],
    )
    render(<Shell />)
    await act(async () => {})

    dragToPane('f1')
    await drainTray()

    expect(deleteChatFolderFn).toHaveBeenCalledWith('w1', 'f1', undefined)
    expect(rowIds()).toEqual(['cf'])
    expect(depthOf('cf')).toBe(0)
    expect(deleteChatFn).not.toHaveBeenCalled()
  })

  it('deleting the ACTIVE tab of a multi-tab pane activates the sibling, not a blank pane', async () => {
    // With two chat tabs in one pane and the FIRST active, deleting it via raw
    // closeBuffer left the pane's activeBufferId pointing at a buffer that no
    // longer exists, so the pane rendered empty until the user clicked the
    // surviving tab. removeBufferFromPane is what activates the sibling.
    seed()
    render(<Shell />)
    fireEvent.click(rowFor('c1'))
    fireEvent.click(rowFor('c2'))
    fireEvent.click(rowFor('c1'))
    const pane = state().activePaneId
    const survivor = agentBuffers().find((b) => b.chatId === 'c2')!.id
    expect(state().panes[pane].bufferIds).toHaveLength(2)

    dragToPane('c1')
    await drainTray()

    expect(state().panes[pane].bufferIds).toEqual([survivor])
    expect(state().panes[pane].activeBufferId).toBe(survivor)
  })

  it('a failed delete snaps the chat back into the list and reopens its pane tab', async () => {
    deleteChatFn.mockRejectedValue(new Error('nope'))
    seed()
    render(<Shell />)
    fireEvent.click(rowFor('c1'))

    dragToPane('c1')
    await drainTray()

    expect(rowIds()).toEqual(['c2', 'c1'])
    // The optimistically closed tab comes back with the chat — and stays active.
    expect(agentBuffers()).toHaveLength(1)
    expect(agentBuffers()[0]).toMatchObject({ type: 'agentChat', chatId: 'c1', name: 'First' })
    expect(state().agentChats.activeChatId).toBe('c1')
    // The user has already walked away from the gesture, so the tray is the only
    // place a refusal can be surfaced.
    expect(toastErrorFn).toHaveBeenCalledWith(expect.stringContaining('First'))
  })

  it('a failed delete brings back the folders that were inside the subtree', async () => {
    deleteChatFn.mockRejectedValue(new Error('nope'))
    seed(
      [
        chat('p1', 'Parent', 'claude', '2026-01-01T00:00:00Z', { order: 0 }),
        chat('t1', 'Thread', 'claude', '2026-01-02T00:00:00Z', { parentId: 'fin', order: 0 }),
      ],
      [folder('fin', 'Inner', { parentId: 'p1' })],
    )
    render(<Shell />)
    await act(async () => {})
    expect(rowIds()).toEqual(['p1', 'fin', 't1'])

    dragToPane('p1')
    expect(rowIds()).toEqual([])
    await drainTray()

    // A folder caught inside a deleted chat's subtree goes with it — and comes
    // back with it, or the chats return to a level they were never filed in.
    expect(rowIds()).toEqual(['p1', 'fin', 't1'])
  })

  it('a failed delete restores a chat that had no tab, without opening one', async () => {
    // The rollback must not invent a tab the chat never had, and must not make a
    // chat active that was not.
    deleteChatFn.mockRejectedValue(new Error('nope'))
    seed()
    render(<Shell />)

    dragToPane('c1')
    await drainTray()

    expect(rowIds()).toEqual(['c2', 'c1'])
    expect(agentBuffers()).toHaveLength(0)
    expect(state().agentChats.activeChatId).not.toBe('c1')
  })

  it('a failed folder delete puts the folder back around its chats', async () => {
    deleteChatFolderFn.mockRejectedValue(new Error('folder not empty'))
    seed(
      [chat('cf', 'Filed', 'claude', '2026-01-05T00:00:00Z', { parentId: 'f1' })],
      [folder('f1', 'Spikes')],
    )
    render(<Shell />)
    await act(async () => {})

    dragToPane('f1')
    await drainTray()

    expect(rowIds()).toEqual(['f1', 'cf'])
    expect(depthOf('cf')).toBe(1)
    expect(toastErrorFn).toHaveBeenCalledWith(expect.stringContaining('Spikes'))
  })

  // ── The menu, which is the same path ──────────────────────────────

  it('deletes from the menu through the same tray', async () => {
    seed(THREADED)
    render(<Shell />)

    fireEvent.contextMenu(rowFor('p1'))
    fireEvent.click(screen.getByText('Delete chat'))

    expect(rowIds()).toEqual([])
    expect(trayIds()).toEqual(['p1'])
    await drainTray()
    expect(deleteChatFn).toHaveBeenCalledTimes(4)
  })

  it('deletes a folder from the menu, promoting its chats', async () => {
    seed(
      [chat('cf', 'Filed', 'claude', '2026-01-05T00:00:00Z', { parentId: 'f1' })],
      [folder('f1', 'Spikes')],
    )
    render(<Shell />)
    await act(async () => {})

    fireEvent.contextMenu(rowFor('f1'))
    fireEvent.click(screen.getByText('Delete folder'))
    await drainTray()

    expect(deleteChatFolderFn).toHaveBeenCalledWith('w1', 'f1', undefined)
    expect(rowIds()).toEqual(['cf'])
    expect(deleteChatFn).not.toHaveBeenCalled()
  })

  it('holds nothing for a row the store no longer has', () => {
    seed()
    render(<Shell />)

    fireEvent.contextMenu(rowFor('c1'))
    // The `deleted` frame lands while the menu is open. A tray row naming
    // nothing is worse than no tray row.
    act(() => state().removeAgentChat('c1'))
    fireEvent.click(screen.getByText('Delete chat'))

    expect(trayIds()).toEqual([])
  })
})
