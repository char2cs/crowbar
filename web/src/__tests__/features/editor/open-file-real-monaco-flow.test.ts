import { describe, expect, it } from 'vitest'

// jsdom has no matchMedia; monaco's StandaloneThemeService requires it at
// editor-create time. Inert stub (never matches, no listeners fire) — an
// environment shim, not behavior under test.
if (typeof window !== 'undefined' && typeof window.matchMedia !== 'function') {
  window.matchMedia = ((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  })) as typeof window.matchMedia
}

// REAL-monaco end-to-end guard for the lazy-arm seam's content path. NO monaco
// mocks anywhere in this file. Walks the exact production sequence at store
// level — openFileContent has already fetched the content, so:
//   1. bufferActions.openContent({type:'editor', content}) (sidebar click)
//   2. store.armEditor()                                    (EditorPane arm gate)
//   3. manager.mountPane(paneId, container)                 (controller mount)
//   4. applyActiveBuffer(...)                                (controller applySwitch)
// and asserts the pane's editor ends up holding a model that CONTAINS the
// buffer's content — through the REAL monaco-adapters (createModel/getModel URI
// resolution, setModel), which every other suite mocks away.
//
// Provenance: written to reproduce a live report of "editor mounts but
// .view-lines stays permanently empty". It PASSED unchanged at that commit
// (b5696475) — correctly: live-instrumentation then proved the model/content/
// layout were all healthy in the running app and the blank view was WKWebView
// suspending requestAnimationFrame while the DISPLAY WAS ASLEEP (monaco's view
// paint is rAF-scheduled; it rendered the instant the display woke). Kept as a
// permanent guard: it pins the arm-seam → real-monaco content path end to end.
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { windowPaneStore, resetWindowPaneStoreForTests } from '@/features/panes/stores/window-pane-store'
import { applyActiveBuffer } from '@/features/editor/lib/pane-editor-controller'
import { fileUri } from '@/features/editor/lib/editor-uri'

const CONTENT = 'const hello = 42\nexport default hello\n'

describe('open file → real monaco model content (live-flow repro)', () => {
  it('the mounted pane editor holds a model containing the opened buffer content', async () => {
    resetWindowPaneStoreForTests()
    const store = createWorkspaceStore('repro-ws')

    // 1. Sidebar click: content is fetched FIRST, then the buffer opens with it.
    // Task 26: panes/buffers are window-level now.
    const bufferId = windowPaneStore.getState().bufferActions.openContent({
      type: 'editor',
      path: 'src/app.ts',
      name: 'app.ts',
      content: CONTENT,
      workspaceId: 'repro-ws',
    })

    // 2. EditorPane's arm gate — block on the real dynamic-import promise.
    await store.armEditor()
    const manager = store.editorManager!

    // 3. Controller mount effect: mount the retained widget into a container.
    const paneId = windowPaneStore.getState().activePaneId
    const container = document.createElement('div')
    document.body.appendChild(container)
    try {
      manager.mountPane(paneId, container)

      // 4. Controller applySwitch → applyActiveBuffer (model swap + publish).
      const state = windowPaneStore.getState()
      const activeBufferId = state.panes[paneId]?.activeEditorTabId
      expect(activeBufferId).toBe(bufferId)
      const buffer = state.buffers.find((b) => b.id === activeBufferId)!
      const ctx = applyActiveBuffer({ manager, registry: store.activeEditorRegistry }, paneId, {
        bufferId: buffer.id,
        filePath: buffer.path,
      })

      // The live symptom: editor sized, but no model/content ever reaches it.
      const editorLike = manager.getEditor(paneId)
      expect(editorLike).toBeDefined()
      const model = editorLike!.getModel()
      expect(model).not.toBeNull()
      expect(model!.getValue()).toBe(CONTENT)
      // Registry bookkeeping agrees (same model under the buffer's uri).
      expect(store.modelRegistry!.get(fileUri(buffer.path))?.getValue()).toBe(CONTENT)
      // And the context published to satellites carries the model.
      expect(ctx?.model).toBeTruthy()
    } finally {
      manager.unmountPane(paneId)
      container.remove()
    }
    // 120s, not 30s. This is a CEILING on a genuinely slow operation, not a
    // wait for state to settle — the test blocks on real promises throughout.
    // It loads REAL monaco (a 3.6MB chunk) through the arm seam's dynamic
    // import. Measured on this machine: ~3.3s alone on an idle box, ~15s alone
    // under load, and it BLEW the old 30s ceiling twice when the full 339-file
    // suite ran in parallel — reproduced deterministically by pairing it with
    // four unrelated editor suites. CI is worse than either measurement: two
    // cores, and `test:coverage` adds v8 instrumentation over that same monaco
    // bundle. A ceiling this test can hit on a busy machine is a flaky gate,
    // and a flaky gate is one people learn to re-run instead of read.
  }, 120000)
})
