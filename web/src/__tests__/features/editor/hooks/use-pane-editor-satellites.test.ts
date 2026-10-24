import { afterEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import { loadEditorHost, type EditorHost } from '@/features/editor/host/editor-host'
import { activeEditors } from '@/features/editor/lib/active-editor-context'
import { fileUri } from '@/features/editor/lib/editor-uri'
import { applyActiveBuffer } from '@/features/editor/lib/pane-editor-controller'
import { usePaneEditorSatellites } from '@/features/editor/hooks/use-pane-editor-satellites'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'

// Both talk to the daemon; neither touches the model text under test.
vi.mock('@/features/editor/hooks/use-lsp-document-sync', () => ({ useLspDocumentSync: () => {} }))
vi.mock('@/features/editor/hooks/use-inline-blame', () => ({ useInlineBlame: () => {} }))

const PATH = 'package.json'
const A = fileUri('ws-a', PATH)
const B = fileUri('ws-b', PATH)

function openSamePathInTwoWorkspaces() {
  resetWindowPaneStoreForTests()
  const open = (workspaceId: string, content: string) =>
    windowPaneStore
      .getState()
      .bufferActions.openContent({ type: 'editor', path: PATH, name: PATH, content, workspaceId })
  const a = open('ws-a', 'AAAA')
  const b = open('ws-b', 'BBBB')
  return { a, b, paneId: windowPaneStore.getState().activePaneId }
}

function activate(paneId: string, bufferId: string) {
  act(() => windowPaneStore.getState().paneActions.activateEditorTabInPane(paneId, bufferId))
}

/** What the pane controller does on a tab switch: show the active buffer's model. */
function showActive(host: EditorHost, paneId: string) {
  const state = windowPaneStore.getState()
  const id = state.panes[paneId]?.activeEditorTabId
  const buffer = state.buffers.find((b) => b.id === id)
  if (!buffer?.path) return
  applyActiveBuffer({ manager: host.manager, registry: activeEditors }, paneId, {
    bufferId: buffer.id,
    filePath: buffer.path,
    workspaceId: buffer.workspaceId,
  })
}

const cleanups: Array<() => void> = []
afterEach(() => {
  for (const undo of cleanups.splice(0).reverse()) undo()
})

async function mountPane() {
  const fixture = openSamePathInTwoWorkspaces()
  const host = await loadEditorHost()
  const container = document.body.appendChild(document.createElement('div'))
  host.manager.mountPane(fixture.paneId, container)
  cleanups.push(() => {
    host.manager.unmountPane(fixture.paneId)
    activeEditors.clear(fixture.paneId)
    container.remove()
  })
  const satellites = () => {
    const view = renderHook(() =>
      usePaneEditorSatellites(fixture.paneId, { registry: activeEditors, manager: host.manager }),
    )
    cleanups.push(view.unmount)
  }
  const followActiveTab = () =>
    cleanups.push(windowPaneStore.subscribe(() => showActive(host, fixture.paneId)))
  return { ...fixture, host, satellites, followActiveTab }
}

describe('usePaneEditorSatellites store → model reconcile', () => {
  it('a tab switch between two workspaces’ same-path files leaves each model its own text', async () => {
    const { a, b, paneId, host, satellites, followActiveTab } = await mountPane()
    activate(paneId, a)
    showActive(host, paneId)
    followActiveTab()
    satellites()

    activate(paneId, b)

    expect(host.models.get(A)?.getValue()).toBe('AAAA')
    expect(host.models.get(B)?.getValue()).toBe('BBBB')
  }, 120000)

  it('binding while the store shows another workspace’s same-path file writes nothing', async () => {
    const { a, b, paneId, host, satellites } = await mountPane()
    activate(paneId, a)
    showActive(host, paneId)
    activate(paneId, b)

    satellites()

    expect(host.models.get(A)?.getValue()).toBe('AAAA')
  }, 120000)
})
