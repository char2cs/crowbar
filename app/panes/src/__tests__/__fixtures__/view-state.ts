import { createStore } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import { initialViewState, makePane, type ViewState } from '@/features/panes/lib/view-state'
import { viewIntegrityViolations } from '@/features/panes/lib/view-integrity'
import { createPaneSlice, type PaneSlice } from '@/features/panes/stores/slices/pane-slice'
import type { EditorTabBase } from '@/features/panes/types/pane-content'
import type { LayoutNode, PaneGroup } from '@/features/panes/types/pane'
import { createLeaf } from '@/features/panes/utils/pane-layout'
import { createSplit } from '@/__tests__/__fixtures__/pane-layout'
import type { WindowPaneStore } from '@/features/panes/stores/window-pane-store'
import { placeTab } from '@/features/panes/stores/slices/pane-actions/editor-tab-actions'
import type { PaneContent } from '@/features/panes/types/pane-content'

export interface PaneSpec {
  id: string
  chatId?: string | null
  runnerId?: string | null
  workspaceId?: string | null
  editorTabIds?: string[]
}

export interface ViewSpec {
  id: string
  projectId?: string
  panes: PaneSpec[]
}

export function row(ids: readonly string[]): LayoutNode {
  let layout: LayoutNode = createLeaf(ids[0])
  for (const id of ids.slice(1)) layout = createSplit('horizontal', layout, createLeaf(id))
  return layout
}

function toPane(spec: PaneSpec, viewId: string | null): PaneGroup {
  return makePane(spec.id, viewId, {
    chatId: spec.chatId ?? null,
    runnerId: spec.runnerId ?? null,
    workspaceId: spec.workspaceId ?? null,
    editorTabIds: spec.editorTabIds ?? [],
    activeEditorTabId: spec.editorTabIds?.[0] ?? null,
    editorOpen: (spec.editorTabIds?.length ?? 0) > 0,
  })
}

export function buildViewState(opts: {
  views?: ViewSpec[]
  stage?: PaneSpec[]
  active?: string | null
  activeProjectId?: string | null
}): ViewState {
  const base = initialViewState()
  const state: ViewState = { ...base, activeProjectId: opts.activeProjectId ?? null }
  if (opts.stage) {
    delete state.panes['root-pane']
    for (const spec of opts.stage) state.panes[spec.id] = toPane(spec, null)
    state.stage = row(opts.stage.map((p) => p.id))
  }
  for (const view of opts.views ?? []) {
    for (const spec of view.panes) state.panes[spec.id] = toPane(spec, view.id)
    state.views[view.id] = {
      id: view.id,
      projectId: view.projectId ?? 'p1',
      layout: row(view.panes.map((p) => p.id)),
    }
    state.viewOrder.push(view.id)
  }
  const active = opts.active === undefined ? (opts.views?.[0]?.id ?? null) : opts.active
  state.activeViewId = active
  const first = active ? opts.views!.find((v) => v.id === active)!.panes[0].id : undefined
  state.activePaneId = first ?? opts.stage?.[0]?.id ?? 'root-pane'
  state.mostRecentActivePaneIds = [state.activePaneId]
  if (active) state.activeViewByProject[state.views[active].projectId] = active
  return state
}

export function seedStore(store: WindowPaneStore, state: ViewState): void {
  store.setState({ ...state })
}

export function rowIds(state: Pick<ViewState, 'viewOrder'>): string[] {
  return [...state.viewOrder]
}

export function makePaneOnlyStore<E extends object = object>(extra?: E) {
  const store = createStore<PaneSlice & E>()(
    immer((set, get, api) => ({
      ...createPaneSlice(...([set, get, api] as unknown as Parameters<typeof createPaneSlice>)),
      ...(extra as E),
    })),
  )
  store.subscribe((state) => assertViewIntegrity(state))
  return store
}

export function assertViewIntegrity(state: ViewState): void {
  const violations = viewIntegrityViolations(state)
  if (violations.length > 0) throw new Error(`view integrity: ${violations.join('; ')}`)
}

export function editorTab(id: string, type: 'editor' | 'terminal' = 'editor'): EditorTabBase {
  return { id, type, name: `${id}.ts`, workspaceId: 'ws-test' } as EditorTabBase
}

export function seedChatPaneRecord(
  store: WindowPaneStore,
  paneId: string,
  chatId: string,
  runnerId: string | null,
  projectId = 'p1',
): void {
  store.setState((s) => {
    const viewId = `view-${paneId}`
    s.panes[paneId] = makePane(paneId, viewId, { chatId, runnerId })
    s.views[viewId] = { id: viewId, projectId, layout: createLeaf(paneId) }
    s.viewOrder = [...s.viewOrder, viewId]
    return s
  })
}

export function seatTab(store: WindowPaneStore, paneId: string, tab: EditorTabBase): void {
  store.setState((s) => {
    if (!s.buffers.some((b) => b.id === tab.id)) {
      s.buffers.push({ isPinned: false, isPreview: false, ...tab } as PaneContent)
    }
    placeTab(s, paneId, tab.id)
    return s
  })
}
