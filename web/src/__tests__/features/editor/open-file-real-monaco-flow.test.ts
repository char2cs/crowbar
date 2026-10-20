import { describe, expect, it } from 'vitest'
import { loadEditorHost } from '@/features/editor/host/editor-host'
import { activeEditors } from '@/features/editor/lib/active-editor-context'
import { fileUri } from '@/features/editor/lib/editor-uri'
import { applyActiveBuffer } from '@/features/editor/lib/pane-editor-controller'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'

// REAL-monaco end-to-end guard for the editor host's content path. NO monaco
// mocks anywhere in this file. Walks the production sequence at store level —
// the content was fetched before the buffer opened, so:
//   1. bufferActions.openContent({type:'editor', content}) (sidebar click)
//   2. loadEditorHost()                                    (EditorPane's host gate)
//   3. manager.mountPane(paneId, container)                (controller mount)
//   4. applyActiveBuffer(...)                              (controller applySwitch)
// and asserts the pane's editor holds a model that CONTAINS the buffer's
// content, through the real monaco-adapters.

const CONTENT = 'const hello = 42\nexport default hello\n'

describe('open file → real monaco model content', () => {
  it('the mounted pane editor holds a model containing the opened buffer content', async () => {
    resetWindowPaneStoreForTests()

    const bufferId = windowPaneStore.getState().bufferActions.openContent({
      type: 'editor',
      path: 'src/app.ts',
      name: 'app.ts',
      content: CONTENT,
      workspaceId: 'repro-ws',
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
      // Set explicitly by openContent's `path: 'src/app.ts'` above.
      const filePath = buffer.path!
      const ctx = applyActiveBuffer({ manager, registry: activeEditors }, paneId, {
        bufferId: buffer.id,
        filePath,
        workspaceId: buffer.workspaceId,
      })

      const model = manager.getEditor(paneId)?.getModel()
      expect(model?.getValue()).toBe(CONTENT)
      // The host's model table agrees (same model under the buffer's uri).
      expect(host.models.get(fileUri('repro-ws', filePath))?.getValue()).toBe(CONTENT)
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
