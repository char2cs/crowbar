import { describe, expect, it } from 'vitest'
import { loadEditorHost } from '@/features/editor/host/editor-host'
import { activeEditors } from '@/features/editor/lib/active-editor-context'
import { documentUri } from '@/features/editor/documents/document-ref'
import { applyActiveBuffer } from '@/features/editor/lib/pane-editor-controller'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import { workspaceRef } from '@/lib/api/documents'

// Real Monaco, no mocks: the production open sequence at store level (open the
// buffer, load the host, mount the pane, apply the active buffer) must leave the
// pane's model holding the buffer's content.

const CONTENT = 'const hello = 42\nexport default hello\n'

describe('open file → real monaco model content', () => {
  it('the mounted pane editor holds a model containing the opened buffer content', async () => {
    resetWindowPaneStoreForTests()

    const bufferId = windowPaneStore.getState().bufferActions.openContent({
      type: 'editor',
      doc: workspaceRef('repro-ws', 'src/app.ts'),
      name: 'app.ts',
      content: CONTENT,
    })

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
      // Set explicitly by openContent's `doc` above.
      const filePath = buffer.path!
      const ctx = applyActiveBuffer({ manager, registry: activeEditors }, paneId, {
        bufferId: buffer.id,
        doc: workspaceRef(buffer.workspaceId, filePath),
      })

      const model = manager.getEditor(paneId)?.getModel()
      expect(model?.getValue()).toBe(CONTENT)
      // The host's model table agrees (same model under the buffer's uri).
      expect(host.models.get(documentUri(workspaceRef('repro-ws', filePath)))?.getValue()).toBe(
        CONTENT,
      )
      // And the context published to satellites carries the model.
      expect(ctx?.model).toBeTruthy()
    } finally {
      manager.unmountPane(paneId)
      container.remove()
    }
    // A ceiling, not a wait: the test blocks on real promises, but the first
    // load pulls the multi-MB monaco chunk, which takes seconds under a
    // parallel suite with coverage instrumentation on a two-core CI box.
  }, 120000)
})
