import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { IEditorLike, MonacoEditorApi } from '@/features/editor/lib/editor-manager'
import type { IModelLike, MonacoModelApi } from '@/features/editor/lib/model-registry'

// Regression: EditorPane used to resolve its EditorManager from the AMBIENT
// WorkspaceStoreContext instead of the BUFFER's own workspaceId. WorkspaceHost
// keeps every retained WorkspaceView mounted for keep-alive, each rendering
// the same window-level pane tree under a DIFFERENT ambient context — a
// wrong-ambient hidden copy would arm/mount against the WRONG workspace's
// manager, leaking a second Monaco model the buffer's own `closeBuffer`
// cleanup (scoped to buf.workspaceId) never visits.
//
// EditorPane now prefers `getWorkspaceStore(buffer.workspaceId)` and falls
// back to the ambient workspace ONLY when the buffer's own workspace has no
// registered store yet (a buffer opened by a chat the user hasn't navigated
// into this session — live-verified in a running dev-desktop instance: without
// this fallback such a tab renders permanently blank instead of merely
// wrong-scoped, since nothing ever calls `getOrCreateWorkspaceStore` for it
// and minting one here would leak a store WorkspaceHost never agreed to own).
vi.mock('@/features/editor/lib/monaco-adapters', () => {
  const fakeModelApi = (): MonacoModelApi => {
    const models = new Map<string, IModelLike>()
    return {
      createModel: (value, _lang, uri) => {
        let text = value
        const m: IModelLike = {
          uri,
          dispose: () => models.delete(uri),
          getValue: () => text,
          setValueIfChanged: (next) => {
            text = next
          },
        }
        models.set(uri, m)
        return m
      },
      getModel: (uri) => models.get(uri) ?? null,
    }
  }
  const fakeEditorApi = (): MonacoEditorApi => ({
    create: (): IEditorLike => ({
      setModel: () => {},
      getModel: () => null,
      saveViewState: () => null,
      restoreViewState: () => {},
      layout: () => {},
      dispose: () => {},
      raw: () => null,
    }),
  })
  return {
    EDITOR_CREATE_OPTIONS: {},
    langForUri: (_uri: string) => 'plaintext',
    realModelApi: fakeModelApi,
    realEditorApi: fakeEditorApi,
  }
})

vi.mock('@/features/editor/components/editor-surface', () => ({
  EditorSurface: (props: { workspaceId: string }) => (
    <div data-testid="monaco-surface" data-workspace-id={props.workspaceId} />
  ),
}))

let currentBuffer: { id: string; type: string; path: string; name: string; workspaceId: string }
vi.mock('@/features/workspace/stores/hooks/use-buffer-store', () => ({
  useBufferById: (bufferId: string) => ({ ...currentBuffer, id: bufferId }),
}))

import { EditorPane } from '@/features/panes/components/editor-pane'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'

function renderInAmbient(ambientWsId: string) {
  const ambientStore = createWorkspaceStore(ambientWsId)
  return render(
    <WorkspaceStoreContext.Provider value={ambientStore}>
      <EditorPane
        paneId="p1"
        bufferId="b1"
        isActiveSurface
        isPreview={false}
        onPromote={() => {}}
      />
    </WorkspaceStoreContext.Provider>,
  )
}

describe('EditorPane — resolves the EditorManager by the buffer’s own workspace', () => {
  it('prefers the buffer’s own workspace store over a DIFFERENT ambient one', async () => {
    const bufferWsId = 'editor-pane-scope-buffer-ws'
    const ambientWsId = 'editor-pane-scope-ambient-ws'
    getOrCreateWorkspaceStore(bufferWsId)
    currentBuffer = {
      id: 'b1',
      type: 'editor',
      path: '/r/main.ts',
      name: 'main.ts',
      workspaceId: bufferWsId,
    }

    renderInAmbient(ambientWsId)

    const surface = await screen.findByTestId('monaco-surface')
    expect(surface.dataset.workspaceId).toBe(bufferWsId)
  })

  it('falls back to the ambient workspace when the buffer’s own workspace has no store yet', async () => {
    const bufferWsId = 'editor-pane-scope-never-mounted-ws'
    const ambientWsId = 'editor-pane-scope-ambient-fallback-ws'
    getOrCreateWorkspaceStore(ambientWsId)
    // Deliberately NOT calling getOrCreateWorkspaceStore(bufferWsId) — this is
    // the "buffer's own WorkspaceView never mounted this session" case.
    currentBuffer = {
      id: 'b1',
      type: 'editor',
      path: '/r/main.ts',
      name: 'main.ts',
      workspaceId: bufferWsId,
    }

    renderInAmbient(ambientWsId)

    const surface = await screen.findByTestId('monaco-surface')
    expect(surface.dataset.workspaceId).toBe(ambientWsId)
  })
})
