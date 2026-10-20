import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import type * as Monaco from 'monaco-editor'
import { getEditorHost, loadEditorHost, type EditorHost } from '@/features/editor/host/editor-host'
import { activeEditors } from '@/features/editor/lib/active-editor-context'
import { fileUri } from '@/features/editor/lib/editor-uri'
import { applyActiveBuffer } from '@/features/editor/lib/pane-editor-controller'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import {
  destroyWorkspaceStore,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'

// Real Monaco, no mocks: the host's whole job is the one model table every
// pane shares. A ceiling, not a wait — the first load pulls the multi-MB
// monaco chunk, which takes seconds on a busy CI box.
const MONACO_LOAD_MS = 120_000

const mounted: string[] = []

function open(workspaceId: string, path: string, content: string): string {
  return windowPaneStore
    .getState()
    .bufferActions.openContent({ type: 'editor', path, name: path, content, workspaceId })
}

function show(host: EditorHost, paneId: string, bufferId: string, ws: string, path: string) {
  if (!mounted.includes(paneId)) {
    host.manager.mountPane(paneId, document.body.appendChild(document.createElement('div')))
    mounted.push(paneId)
  }
  applyActiveBuffer({ manager: host.manager, registry: activeEditors }, paneId, {
    bufferId,
    filePath: path,
    workspaceId: ws,
  })
}

function shownModel(host: EditorHost, paneId: string): Monaco.editor.ITextModel | null {
  return (host.manager.getRawEditor(paneId) as Monaco.editor.ICodeEditor).getModel()
}

beforeEach(() => {
  resetWindowPaneStoreForTests()
})

afterEach(() => {
  for (const paneId of mounted.splice(0)) getEditorHost()?.manager.unmountPane(paneId)
  document.body.replaceChildren()
})

describe('editor host', () => {
  it(
    'hands every caller the same host',
    async () => {
      const [a, b] = await Promise.all([loadEditorHost(), loadEditorHost()])
      expect(a).toBe(b)
      expect(getEditorHost()).toBe(a)
    },
    MONACO_LOAD_MS,
  )

  it(
    'keeps two workspaces’ files at the same path apart',
    async () => {
      const host = await loadEditorHost()
      const a = open('ws-a', 'src/x.ts', 'A')
      const b = open('ws-b', 'src/x.ts', 'B')

      show(host, 'p1', a, 'ws-a', 'src/x.ts')
      show(host, 'p2', b, 'ws-b', 'src/x.ts')

      expect(shownModel(host, 'p1')?.getValue()).toBe('A')
      expect(shownModel(host, 'p2')?.getValue()).toBe('B')
    },
    MONACO_LOAD_MS,
  )

  it(
    'a pane keeps its model when the buffer’s workspace store is destroyed',
    async () => {
      const host = await loadEditorHost()
      getOrCreateWorkspaceStore('ws-a')
      const a = open('ws-a', 'src/x.ts', 'A')
      show(host, 'p1', a, 'ws-a', 'src/x.ts')

      destroyWorkspaceStore('ws-a')

      expect(host.models.get(fileUri('ws-a', 'src/x.ts'))?.getValue()).toBe('A')
      expect(shownModel(host, 'p1')?.isDisposed()).toBe(false)
    },
    MONACO_LOAD_MS,
  )

  it(
    'a pane keeps its model when another pane closes the same file',
    async () => {
      const host = await loadEditorHost()
      const a = open('ws-a', 'src/x.ts', 'A')
      show(host, 'p1', a, 'ws-a', 'src/x.ts')
      show(host, 'p2', a, 'ws-a', 'src/x.ts')

      host.manager.closeBuffer('p2', fileUri('ws-a', 'src/x.ts'))

      const model = shownModel(host, 'p1')
      expect(model?.isDisposed()).toBe(false)
      expect(model?.getValue()).toBe('A')
    },
    MONACO_LOAD_MS,
  )
})
