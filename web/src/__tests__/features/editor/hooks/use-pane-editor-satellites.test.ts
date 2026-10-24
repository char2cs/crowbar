import { afterEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import { editor as monacoEditor, languages, Uri } from 'monaco-editor/esm/vs/editor/editor.api.js'
import { MONACO_LOAD_MS, openTestDocument } from '@/__tests__/__fixtures__/documents'
import { loadEditorHost, type EditorHost } from '@/features/editor/host/editor-host'
import { activeEditors } from '@/features/editor/lib/active-editor-context'
import { documentRefOf, documentUri } from '@/features/editor/documents/document-ref'
import { openUntitledDocument } from '@/features/editor/documents/open-document'
import { workspaceRef } from '@/lib/api/documents'
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
const A = documentUri(workspaceRef('ws-a', PATH))

async function openSamePathInTwoWorkspaces() {
  resetWindowPaneStoreForTests()
  const a = await openTestDocument(workspaceRef('ws-a', PATH), 'AAAA')
  const b = await openTestDocument(workspaceRef('ws-b', PATH), 'BBBB')
  return { a, b, paneId: windowPaneStore.getState().activePaneId }
}

function overrideLanguage(bufferId: string, languageOverride: string) {
  act(() =>
    windowPaneStore.setState((state) => ({
      buffers: state.buffers.map((buf) =>
        buf.id === bufferId && buf.type === 'editor' ? { ...buf, languageOverride } : buf,
      ),
    })),
  )
}

function activate(paneId: string, bufferId: string) {
  act(() => windowPaneStore.getState().paneActions.activateEditorTabInPane(paneId, bufferId))
}

/** What the pane controller does on a tab switch: show the active buffer's model. */
function showActive(host: EditorHost, paneId: string) {
  const state = windowPaneStore.getState()
  const id = state.panes[paneId]?.activeEditorTabId
  const buffer = state.buffers.find((b) => b.id === id)
  if (!buffer || buffer.type !== 'editor') return
  applyActiveBuffer({ manager: host.manager, registry: activeEditors }, paneId, {
    doc: documentRefOf(buffer),
  })
}

const cleanups: Array<() => void> = []
afterEach(() => {
  for (const undo of cleanups.splice(0).reverse()) undo()
})

async function mountPane() {
  const fixture = await openSamePathInTwoWorkspaces()
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
      usePaneEditorSatellites(fixture.paneId, { registry: activeEditors }),
    )
    cleanups.push(view.unmount)
  }
  return { ...fixture, host, satellites }
}

describe('usePaneEditorSatellites model language', () => {
  it(
    'the bound model keeps its own language while the active tab overrides another’s',
    async () => {
      languages.register({ id: 'json' })
      languages.register({ id: 'python' })
      const { a, b, paneId, host, satellites } = await mountPane()
      activate(paneId, a)
      showActive(host, paneId)
      activate(paneId, b)
      overrideLanguage(b, 'python')

      satellites()

      expect(monacoEditor.getModel(Uri.parse(A))?.getLanguageId()).toBe('json')
    },
    MONACO_LOAD_MS,
  )

  it(
    'an untitled buffer’s chosen language reaches its model',
    async () => {
      languages.register({ id: 'python' })
      const { paneId, host, satellites } = await mountPane()
      const untitled = await openUntitledDocument('ws-a', paneId)
      showActive(host, paneId)
      satellites()

      overrideLanguage(untitled, 'python')

      const uri = documentUri(workspaceRef('ws-a', 'untitled:Untitled-1'))
      expect(monacoEditor.getModel(Uri.parse(uri))?.getLanguageId()).toBe('python')
    },
    MONACO_LOAD_MS,
  )
})
