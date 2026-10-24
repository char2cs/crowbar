import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  applyActiveBuffer,
  type PaneSwitchManager,
} from '@/features/editor/lib/pane-editor-controller'
import type { ActiveEditorRegistry } from '@/features/editor/lib/active-editor-context'
import { documentUri } from '@/features/editor/documents/document-ref'
import { workspaceRef } from '@/lib/api/documents'

/** Fake manager: every document but `closed` is open; getRawEditor exposes the
 *  current model so applyActiveBuffer can read it for the published context. */
function fakeManager(closed?: string) {
  let currentUri: string | null = null
  const models = new Map<string, { uri: string; getValue: () => string }>()
  const editor = { getModel: () => (currentUri ? (models.get(currentUri) ?? null) : null) }
  const manager: PaneSwitchManager & { editor: typeof editor } = {
    editor,
    showBuffer: vi.fn((_paneId: string, uri: string) => {
      if (uri === closed) {
        currentUri = null
        return false
      }
      if (!models.has(uri)) models.set(uri, { uri, getValue: () => '' })
      currentUri = uri
      return true
    }),
    getRawEditor: vi.fn(() => editor),
  }
  return manager
}

// The registry is a window singleton; a fresh module instance per test isolates it.
let freshRegistry: ActiveEditorRegistry
beforeEach(async () => {
  vi.resetModules()
  freshRegistry = (await import('@/features/editor/lib/active-editor-context')).activeEditors
})

describe('applyActiveBuffer', () => {
  it('calls showBuffer(paneId, documentUri(doc)) and publishes context to the registry', () => {
    const manager = fakeManager()
    const registry = freshRegistry
    const setSpy = vi.spyOn(registry, 'set')

    const ctx = applyActiveBuffer({ manager, registry }, 'p1', {
      doc: workspaceRef('w1', '/a.ts'),
    })

    expect(manager.showBuffer).toHaveBeenCalledWith('p1', documentUri(workspaceRef('w1', '/a.ts')))
    expect(setSpy).toHaveBeenCalledTimes(1)
    expect(ctx).toMatchObject({
      paneId: 'p1',
      uri: documentUri(workspaceRef('w1', '/a.ts')),
      filePath: '/a.ts',
    })
    expect(ctx?.model).toBe(manager.editor.getModel())
    expect(ctx?.editor).toBe(manager.editor)
    expect(registry.get('p1')?.uri).toBe(documentUri(workspaceRef('w1', '/a.ts')))
  })

  it('is a no-op when the buffer is null', () => {
    const manager = fakeManager()
    const registry = freshRegistry
    const setSpy = vi.spyOn(registry, 'set')

    const ctx = applyActiveBuffer({ manager, registry }, 'p1', null)

    expect(ctx).toBeUndefined()
    expect(manager.showBuffer).not.toHaveBeenCalled()
    expect(setSpy).not.toHaveBeenCalled()
  })

  it('switching to the SAME buffer does not re-notify subscribers', () => {
    const manager = fakeManager()
    const registry = freshRegistry
    applyActiveBuffer({ manager, registry }, 'p1', {
      doc: workspaceRef('w1', '/a.ts'),
    })

    const cb = vi.fn()
    registry.subscribe('p1', cb) // immediate call (1)
    expect(cb).toHaveBeenCalledTimes(1)

    // Same path again — showBuffer is uri-deduped and registry.set is uri-deduped.
    applyActiveBuffer({ manager, registry }, 'p1', {
      doc: workspaceRef('w1', '/a.ts'),
    })
    expect(cb).toHaveBeenCalledTimes(1) // no re-notify
  })

  it('returns undefined (and does not publish) when no raw editor/model is available', () => {
    const registry = freshRegistry
    const setSpy = vi.spyOn(registry, 'set')
    const manager: PaneSwitchManager = {
      showBuffer: vi.fn(() => true),
      getRawEditor: vi.fn(() => null),
    }

    const ctx = applyActiveBuffer({ manager, registry }, 'p1', {
      doc: workspaceRef('w1', '/a.ts'),
    })

    expect(manager.showBuffer).toHaveBeenCalledWith('p1', documentUri(workspaceRef('w1', '/a.ts')))
    expect(ctx).toBeUndefined()
    expect(setSpy).not.toHaveBeenCalled()
  })

  it('publishes nothing while the document is not open', () => {
    const doc = workspaceRef('w1', '/a.ts')
    const registry = freshRegistry
    const setSpy = vi.spyOn(registry, 'set')

    const ctx = applyActiveBuffer({ manager: fakeManager(documentUri(doc)), registry }, 'p1', {
      doc,
    })

    expect(ctx).toBeUndefined()
    expect(setSpy).not.toHaveBeenCalled()
  })
})
