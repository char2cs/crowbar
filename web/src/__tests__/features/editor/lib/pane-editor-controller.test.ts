import { describe, expect, it, vi } from 'vitest'
import {
  applyActiveBuffer,
  type PaneSwitchManager,
} from '@/features/editor/lib/pane-editor-controller'
import { createActiveEditorRegistry } from '@/features/editor/lib/active-editor-context'
import { fileUri } from '@/features/editor/lib/editor-uri'

/** Fake manager: one model per uri it has been shown; getRawEditor exposes the
 *  current model so applyActiveBuffer can read it for the published context. */
function fakeManager() {
  let currentUri: string | null = null
  const models = new Map<string, { uri: string; getValue: () => string }>()
  const editor = { getModel: () => (currentUri ? (models.get(currentUri) ?? null) : null) }
  const manager: PaneSwitchManager & { editor: typeof editor } = {
    editor,
    showBuffer: vi.fn((_paneId: string, uri: string) => {
      if (currentUri === uri) return
      if (!models.has(uri)) models.set(uri, { uri, getValue: () => '' })
      currentUri = uri
    }),
    getRawEditor: vi.fn(() => editor),
  }
  return manager
}

describe('applyActiveBuffer', () => {
  it('calls showBuffer(paneId, fileUri(path)) and publishes context to the registry', () => {
    const manager = fakeManager()
    const registry = createActiveEditorRegistry()
    const setSpy = vi.spyOn(registry, 'set')

    const ctx = applyActiveBuffer({ manager, registry }, 'p1', { filePath: '/a.ts' })

    expect(manager.showBuffer).toHaveBeenCalledWith('p1', fileUri('/a.ts'))
    expect(setSpy).toHaveBeenCalledTimes(1)
    expect(ctx).toMatchObject({
      paneId: 'p1',
      uri: fileUri('/a.ts'),
      filePath: '/a.ts',
    })
    expect(ctx?.model).toBe(manager.editor.getModel())
    expect(ctx?.editor).toBe(manager.editor)
    expect(registry.get('p1')?.uri).toBe(fileUri('/a.ts'))
  })

  it('is a no-op when the buffer is null', () => {
    const manager = fakeManager()
    const registry = createActiveEditorRegistry()
    const setSpy = vi.spyOn(registry, 'set')

    const ctx = applyActiveBuffer({ manager, registry }, 'p1', null)

    expect(ctx).toBeUndefined()
    expect(manager.showBuffer).not.toHaveBeenCalled()
    expect(setSpy).not.toHaveBeenCalled()
  })

  it('switching to the SAME buffer does not re-notify subscribers', () => {
    const manager = fakeManager()
    const registry = createActiveEditorRegistry()
    applyActiveBuffer({ manager, registry }, 'p1', { filePath: '/a.ts' })

    const cb = vi.fn()
    registry.subscribe('p1', cb) // immediate call (1)
    expect(cb).toHaveBeenCalledTimes(1)

    // Same path again — showBuffer is uri-deduped and registry.set is uri-deduped.
    applyActiveBuffer({ manager, registry }, 'p1', { filePath: '/a.ts' })
    expect(cb).toHaveBeenCalledTimes(1) // no re-notify
  })

  it('returns undefined (and does not publish) when no raw editor/model is available', () => {
    const registry = createActiveEditorRegistry()
    const setSpy = vi.spyOn(registry, 'set')
    const manager: PaneSwitchManager = {
      showBuffer: vi.fn(),
      getRawEditor: vi.fn(() => null),
    }

    const ctx = applyActiveBuffer({ manager, registry }, 'p1', { filePath: '/a.ts' })

    expect(manager.showBuffer).toHaveBeenCalledWith('p1', fileUri('/a.ts'))
    expect(ctx).toBeUndefined()
    expect(setSpy).not.toHaveBeenCalled()
  })
})
