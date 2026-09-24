// Seeded, deterministic sequences of every row action in the spec's table
// plus every non-row event. After each step the invariants hold, and the set
// of rows moved only if the step was a row action.
import { describe, it, expect, vi, afterEach } from 'vitest'
import { assertViewIntegrity } from '@/features/panes/lib/view-integrity'
import { viewChatIds, type ViewState } from '@/features/panes/lib/view-state'
import { restoreWindowPaneState } from '@/lib/persistence/hydrate'
import {
  destroyWorkspaceStore,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import { getAllLeafIds } from '@/features/panes/utils/pane-layout'
import { editorTab, makePaneOnlyStore } from '@/__tests__/__fixtures__/view-state'
import type { WorkspaceLayout } from '@/lib/persistence/schemas'

vi.mock('@/features/panes/lib/release-closed-chat', () => ({
  releaseClosedChat: vi.fn(async () => {}),
}))

afterEach(() => destroyWorkspaceStore('ws-seq'))

/** mulberry32 — tiny, seedable, good enough to walk a state space. */
function prng(seed: number): () => number {
  let a = seed >>> 0
  return () => {
    a = (a + 0x6d2b79f5) >>> 0
    let t = a
    t = Math.imul(t ^ (t >>> 15), t | 1)
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61)
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

type Store = ReturnType<typeof makePaneOnlyStore>
type Step = { name: string; row: boolean; run: () => void }

const CHATS = Array.from({ length: 8 }, (_, i) => `chat-${i}`)
const PROJECTS = ['p1', 'p2']
const ZONES = ['center', 'left', 'right', 'top', 'bottom'] as const

function rowSet(state: ViewState): string {
  return [...state.viewOrder].sort().join(',')
}

function snapshot(s: ViewState): WorkspaceLayout {
  // The JSON round-trip IS the thing under test: persistence stores this layout as
  // JSON, so structuredClone would keep exactly what a real reload drops.
  // react-doctor-disable-next-line no-json-parse-stringify-clone
  return JSON.parse(
    JSON.stringify({
      workspaceId: 'window',
      panes: s.panes,
      views: s.views,
      viewOrder: s.viewOrder,
      activeViewId: s.activeViewId,
      activeViewByProject: s.activeViewByProject,
      stage: s.stage,
      bottomLayout: s.bottomLayout,
      activePaneId: s.activePaneId,
      mostRecentActivePaneIds: s.mostRecentActivePaneIds,
      buffers: [],
      sidebarWidth: 0,
      rightSidebarWidth: 0,
      updatedAt: 0,
    }),
  ) as WorkspaceLayout
}

/** Hydrate a snapshot with one record corrupted: only that record may go. */
function corruptRoundTrip(store: Store, rand: () => number): void {
  const s = store.getState()
  const layout = snapshot(s)
  const victim = s.viewOrder[Math.floor(rand() * s.viewOrder.length)]
  const kind = Math.floor(rand() * 5)
  if (victim) {
    const leaves = getAllLeafIds(layout.views![victim].layout)
    if (kind === 0) for (const id of leaves) layout.panes[id].chatId = null
    if (kind === 1) delete layout.panes[leaves[0]]
    if (kind === 2) layout.panes[leaves[0]].viewId = 'elsewhere'
  }
  if (kind === 3) layout.viewOrder = [...(layout.viewOrder ?? []), 'bogus', 'bogus']
  if (kind === 4) layout.activeViewId = 'bogus'
  const state = restoreWindowPaneState(layout)
  assertViewIntegrity(state)
  const survives = (id: string) =>
    id !== victim || kind > 2 || (kind > 0 && viewChatIds(state, id).length > 0)
  const expected = s.viewOrder.filter(survives)
  expect(state.viewOrder).toEqual(expected)
  store.setState({ ...state, activeProjectId: s.activeProjectId })
}

function roundTrip(store: Store): void {
  const s = store.getState()
  const state = restoreWindowPaneState(snapshot(s))
  store.setState({ ...state, activeProjectId: s.activeProjectId })
}

function steps(store: Store, rand: () => number): Step[] {
  const pick = <T>(xs: readonly T[]): T | undefined => xs[Math.floor(rand() * xs.length)]
  const s = store.getState()
  const a = s.paneActions
  const paneIds = Object.keys(s.panes)
  const chatPanes = paneIds.filter((id) => s.panes[id].chatId)
  const chat = pick(CHATS)!
  const pane = pick(paneIds)!
  const chatPane = pick(chatPanes)
  const view = pick(s.viewOrder)
  const project = pick(PROJECTS)!
  return [
    {
      name: `openChat ${chat} ${project}`,
      row: true,
      run: () => a.openChat(chat, { projectId: project }),
    },
    { name: `openChat ${chat}`, row: true, run: () => a.openChat(chat) },
    {
      name: `dropChatOnPane ${chat} ${pane}`,
      row: true,
      run: () => a.dropChatOnPane(chat, pane, pick(ZONES)!),
    },
    { name: `detachPane ${chatPane}`, row: true, run: () => chatPane && a.detachPane(chatPane) },
    { name: `closePane ${pane}`, row: true, run: () => a.closePane(pane) },
    { name: `closeView ${view}`, row: true, run: () => view && a.closeView(view) },
    { name: `forgetChat ${chat}`, row: true, run: () => a.forgetChat(chat) },
    {
      name: `retargetPane ${chatPane} ${chat}`,
      row: true,
      run: () => chatPane && a.retargetPane(chatPane, chat, 'runner'),
    },
    {
      name: `adoptBackgroundChat ${chat}`,
      row: true,
      run: () => a.adoptBackgroundChat(chat, project),
    },
    {
      name: `reorderView ${view}`,
      row: false,
      run: () => view && a.reorderView(view, pick(s.viewOrder)!, rand() < 0.5 ? 'before' : 'after'),
    },
    { name: `setPaneRunner ${pane}`, row: false, run: () => a.setPaneRunner(pane, 'r2') },
    { name: `setActiveProject ${project}`, row: false, run: () => a.setActiveProject(project) },
    { name: `setActivePane ${pane}`, row: false, run: () => a.setActivePane(pane) },
    { name: `activateView ${view}`, row: false, run: () => view && a.activateView(view) },
    { name: `splitPane ${pane}`, row: false, run: () => a.splitPane(pane, 'horizontal', 'tab-s') },
    {
      name: `addEditorTab ${pane}`,
      row: false,
      run: () => a.addEditorTabToPane(pane, editorTab('tab-x')),
    },
    {
      name: `removeEditorTab ${pane}`,
      row: false,
      run: () => a.removeEditorTabFromPane(pane, pick(s.panes[pane].editorTabIds) ?? 'none'),
    },
    {
      // Keep-alive eviction / workspace unmount: a workspace store comes and goes.
      name: 'workspace unmount',
      row: false,
      run: () => {
        getOrCreateWorkspaceStore('ws-seq')
        destroyWorkspaceStore('ws-seq')
      },
    },
    // A reconnect seed that finds every chat still present changes nothing.
    { name: 'reconnect seed', row: false, run: () => {} },
    { name: 'hydrate round-trip', row: false, run: () => roundTrip(store) },
    { name: 'corrupt hydrate', row: true, run: () => corruptRoundTrip(store, rand) },
  ]
}

function runSequence(seed: number, length: number): void {
  const rand = prng(seed)
  const store = makePaneOnlyStore()
  for (let i = 0; i < length; i++) {
    const options = steps(store, rand)
    const step = options[Math.floor(rand() * options.length)]
    const before = store.getState()
    const label = `seed ${seed} step ${i}: ${step.name}`
    try {
      step.run()
    } catch (err) {
      throw new Error(`${label} threw: ${(err as Error).message}`, { cause: err })
    }
    const after = store.getState()
    assertViewIntegrity(after)
    // Focus is derived from pane writes: it always names a pane that exists.
    expect(after.panes[after.activePaneId], label).toBeDefined()
    if (!step.row) expect(rowSet(after), label).toBe(rowSet(before))
    // Invariant 5: a pane's chat never changes to a different chat except by
    // retargetPane.
    if (!step.name.startsWith('retargetPane')) {
      for (const [id, pane] of Object.entries(after.panes)) {
        const prior = before.panes[id]?.chatId
        if (prior && pane.chatId) expect(pane.chatId, label).toBe(prior)
      }
    }
    for (const viewId of after.viewOrder) {
      expect(viewChatIds(after, viewId).length, label).toBeGreaterThan(0)
    }
  }
}

describe('view actions — seeded sequences', () => {
  const SEEDS = [1, 2, 3, 5, 8, 13, 21, 34, 55, 89, 144, 233]
  for (const seed of SEEDS) {
    it(`seed ${seed}: invariants hold and only row actions move rows`, () => {
      runSequence(seed, 400)
    })
  }
})
