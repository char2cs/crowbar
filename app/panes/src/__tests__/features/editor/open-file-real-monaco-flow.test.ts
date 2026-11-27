import { describe, expect, it } from 'vitest'
import { MONACO_LOAD_MS, openTestDocument } from '@/__tests__/__fixtures__/documents'
import { loadEditorHost } from '@/features/editor/host/editor-host'
import { activeEditors } from '@/features/editor/lib/active-editor-context'
import { documentUri } from '@/features/editor/documents/document-ref'
import { applyActiveBuffer } from '@/features/editor/lib/pane-editor-controller'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { resetWindowPaneStoreForTests } from '@/__tests__/__fixtures__/window-pane-store'
import { workspaceRef } from '@/lib/api/documents'

const CONTENT = 'const hello = 42\nexport default hello\n'

describe('open file → real monaco model content', () => {
  it(
    'the mounted pane editor holds a model containing the opened buffer content',
    async () => {
      resetWindowPaneStoreForTests()

      const bufferId = await openTestDocument(workspaceRef('repro-ws', 'src/app.ts'), CONTENT)

      const host = await loadEditorHost()
      const { manager } = host

      const paneId = windowPaneStore.getState().activePaneId
      const container = document.createElement('div')
      document.body.appendChild(container)
      try {
        manager.mountPane(paneId, container)

        const state = windowPaneStore.getState()
        const activeBufferId = state.panes[paneId]?.activeEditorTabId
        expect(activeBufferId).toBe(bufferId)
        const buffer = state.buffers.find((b) => b.id === activeBufferId)!
        const filePath = buffer.path!
        const ctx = applyActiveBuffer({ manager, registry: activeEditors }, paneId, {
          doc: workspaceRef(buffer.workspaceId, filePath),
        })

        const model = manager.getEditor(paneId)?.getModel()
        expect(model?.getValue()).toBe(CONTENT)
        const own = host.documents.get(documentUri(workspaceRef('repro-ws', filePath)))
        expect(model?.uri).toBe(own?.uri)
        expect(ctx?.model).toBeTruthy()
      } finally {
        manager.unmountPane(paneId)
        container.remove()
      }
    },
    MONACO_LOAD_MS,
  )
})
