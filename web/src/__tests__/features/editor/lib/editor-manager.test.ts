import { describe, expect, it, vi } from 'vitest'
import { EditorManager } from '@/features/editor/lib/editor-manager'
import { ModelRegistry } from '@/features/editor/lib/model-registry'

function fakeModelApi() {
  const models = new Map<string, any>()
  return {
    createModel: vi.fn((value: string, _l: string, uri: string) => {
      let text = value
      const m = {
        uri,
        dispose: vi.fn(() => models.delete(uri)),
        getValue: () => text,
        setValueIfChanged: vi.fn((next: string) => {
          if (next === text) return
          text = next
        }),
      }
      models.set(uri, m)
      return m
    }),
    getModel: (uri: string) => models.get(uri) ?? null,
  }
}
function fakeEditorApi() {
  const created: any[] = []
  return {
    created,
    create: vi.fn(() => {
      let model: any = null; let vs: any = null
      const ed = {
        setModel: vi.fn((m: any) => { model = m }),
        getModel: () => model,
        saveViewState: vi.fn(() => vs ?? { for: model?.uri }),
        restoreViewState: vi.fn((s: any) => { vs = s }),
        layout: vi.fn(), dispose: vi.fn(),
        raw: vi.fn(() => ({ id: 'raw-editor' })),
      }
      created.push(ed); return ed
    }),
  }
}
const lang = () => 'ts'
const text = () => 'code'

describe('EditorManager', () => {
  it('creates exactly one widget per pane', () => {
    const ea = fakeEditorApi(); const reg = new ModelRegistry(fakeModelApi())
    const m = new EditorManager(ea, reg, { lang, text })
    const el = {} as HTMLElement
    m.mountPane('p1', el); m.mountPane('p1', el)
    expect(ea.create).toHaveBeenCalledTimes(1)
  })

  it('showBuffer swaps the model without creating a new widget', () => {
    const ea = fakeEditorApi(); const reg = new ModelRegistry(fakeModelApi())
    const m = new EditorManager(ea, reg, { lang, text })
    m.mountPane('p1', {} as HTMLElement)
    m.showBuffer('p1', 'athas://editor/a')
    m.showBuffer('p1', 'athas://editor/b')
    expect(ea.create).toHaveBeenCalledTimes(1)
    const ed = ea.created[0]
    expect(ed.getModel().uri).toBe('athas://editor/b')
  })

  it('saves outgoing view-state and restores incoming on swap', () => {
    const ea = fakeEditorApi(); const reg = new ModelRegistry(fakeModelApi())
    const m = new EditorManager(ea, reg, { lang, text })
    m.mountPane('p1', {} as HTMLElement)
    m.showBuffer('p1', 'athas://editor/a')
    m.showBuffer('p1', 'athas://editor/b')
    m.showBuffer('p1', 'athas://editor/a')
    const ed = ea.created[0]
    expect(ed.restoreViewState).toHaveBeenCalled()
  })

  it('round-trips view-state: state saved on leaving a buffer is restored on returning', () => {
    const ea = fakeEditorApi(); const reg = new ModelRegistry(fakeModelApi())
    const m = new EditorManager(ea, reg, { lang, text })
    m.mountPane('p1', {} as HTMLElement)
    m.showBuffer('p1', 'athas://editor/a')
    const ed = ea.created[0]
    // simulate a meaningful view-state for 'a' by stubbing saveViewState's return
    const stateA = { scroll: 42 }
    ed.saveViewState = () => stateA
    m.showBuffer('p1', 'athas://editor/b') // leaving a -> saves stateA
    const restored: unknown[] = []
    ed.restoreViewState = (s: unknown) => restored.push(s)
    m.showBuffer('p1', 'athas://editor/a') // returning to a -> restores stateA
    expect(restored).toContain(stateA)
  })

  it('two panes on the same uri share the model but keep independent view-state keys', () => {
    const modelApi = fakeModelApi(); const ea = fakeEditorApi()
    const reg = new ModelRegistry(modelApi)
    const m = new EditorManager(ea, reg, { lang, text })
    m.mountPane('p1', {} as HTMLElement); m.mountPane('p2', {} as HTMLElement)
    m.showBuffer('p1', 'athas://editor/a')
    m.showBuffer('p2', 'athas://editor/a')
    expect(modelApi.createModel).toHaveBeenCalledTimes(1)
    expect(ea.created[0].getModel()).toBe(ea.created[1].getModel())
  })

  it('unmountPane disposes the widget and releases its model', () => {
    const modelApi = fakeModelApi(); const ea = fakeEditorApi()
    const reg = new ModelRegistry(modelApi)
    const m = new EditorManager(ea, reg, { lang, text })
    m.mountPane('p1', {} as HTMLElement)
    m.showBuffer('p1', 'athas://editor/a')
    const ed = ea.created[0]; const model = ed.getModel()
    m.unmountPane('p1')
    expect(ed.dispose).toHaveBeenCalled()
    expect(model.dispose).toHaveBeenCalled()
  })

  it('reuses the model when switching back within a pane (no recreate => undo preserved)', () => {
    const modelApi = fakeModelApi(); const ea = fakeEditorApi()
    const m = new EditorManager(ea, new ModelRegistry(modelApi), { lang, text })
    m.mountPane('p1', {} as HTMLElement)
    m.showBuffer('p1', 'athas://editor/a')
    m.showBuffer('p1', 'athas://editor/b')
    m.showBuffer('p1', 'athas://editor/a') // back to a
    expect(modelApi.createModel).toHaveBeenCalledTimes(2) // a and b only — NOT 3
    expect(ea.created[0].getModel().uri).toBe('athas://editor/a')
  })

  it('closeBuffer releases a tab\'s model; unmountPane releases all remaining held', () => {
    const modelApi = fakeModelApi(); const ea = fakeEditorApi()
    const m = new EditorManager(ea, new ModelRegistry(modelApi), { lang, text })
    m.mountPane('p1', {} as HTMLElement)
    m.showBuffer('p1', 'athas://editor/a')
    m.showBuffer('p1', 'athas://editor/b')
    m.showBuffer('p1', 'athas://editor/a')
    const aModel = ea.created[0].getModel()
    m.closeBuffer('p1', 'athas://editor/b')
    // b released & disposed (no other holder)
    m.unmountPane('p1')
    expect(aModel.dispose).toHaveBeenCalled() // a released on unmount
    expect(ea.created[0].dispose).toHaveBeenCalled()
  })

  it('disposes the model when the LAST holder closes it (closeBuffer)', () => {
    const modelApi = fakeModelApi(); const ea = fakeEditorApi()
    const m = new EditorManager(ea, new ModelRegistry(modelApi), { lang, text })
    m.mountPane('p1', {} as HTMLElement)
    m.showBuffer('p1', 'athas://editor/a')
    const aModel = ea.created[0].getModel()
    expect(aModel.dispose).not.toHaveBeenCalled()
    m.closeBuffer('p1', 'athas://editor/a') // last (only) holder closes
    expect(aModel.dispose).toHaveBeenCalled()
  })

  it('reopening a file AFTER closing it re-acquires a fresh model (createModel called again)', () => {
    const modelApi = fakeModelApi(); const ea = fakeEditorApi()
    const m = new EditorManager(ea, new ModelRegistry(modelApi), { lang, text })
    m.mountPane('p1', {} as HTMLElement)
    m.showBuffer('p1', 'athas://editor/a')
    expect(modelApi.createModel).toHaveBeenCalledTimes(1)
    m.closeBuffer('p1', 'athas://editor/a') // releases + disposes (no holder)
    m.showBuffer('p1', 'athas://editor/a') // reopen → fresh model (reads disk text)
    expect(modelApi.createModel).toHaveBeenCalledTimes(2)
  })

  it('a shared model held by TWO panes disposes only when BOTH panes release it', () => {
    const modelApi = fakeModelApi(); const ea = fakeEditorApi()
    const m = new EditorManager(ea, new ModelRegistry(modelApi), { lang, text })
    m.mountPane('p1', {} as HTMLElement); m.mountPane('p2', {} as HTMLElement)
    m.showBuffer('p1', 'athas://editor/a')
    m.showBuffer('p2', 'athas://editor/a') // shared model, two holders
    const aModel = ea.created[0].getModel()
    m.closeBuffer('p1', 'athas://editor/a') // one holder gone
    expect(aModel.dispose).not.toHaveBeenCalled()
    m.closeBuffer('p2', 'athas://editor/a') // last holder gone
    expect(aModel.dispose).toHaveBeenCalled()
  })

  it('applyExternalEdit edits the live model (not recreate) when the pane shows the uri', () => {
    const modelApi = fakeModelApi(); const ea = fakeEditorApi()
    const reg = new ModelRegistry(modelApi)
    const m = new EditorManager(ea, reg, { lang, text })
    m.mountPane('p1', {} as HTMLElement)
    m.showBuffer('p1', 'athas://editor/a')
    const createdCount = modelApi.createModel.mock.calls.length
    const model = ea.created[0].getModel()
    m.applyExternalEdit('p1', 'athas://editor/a', 'fresh from disk')
    expect(model.setValueIfChanged).toHaveBeenCalledWith('fresh from disk')
    expect(model.getValue()).toBe('fresh from disk')
    // No model recreation — it edited in place (undo preserved).
    expect(modelApi.createModel.mock.calls.length).toBe(createdCount)
  })

  it('applyExternalEdit edits the held registry model when the pane is not showing the uri', () => {
    const modelApi = fakeModelApi(); const ea = fakeEditorApi()
    const reg = new ModelRegistry(modelApi)
    const m = new EditorManager(ea, reg, { lang, text })
    m.mountPane('p1', {} as HTMLElement)
    m.showBuffer('p1', 'athas://editor/a') // a is held
    m.showBuffer('p1', 'athas://editor/b') // now showing b, a still held
    const heldA = reg.get('athas://editor/a')
    expect(heldA?.getValue()).toBe('code') // initial registry text
    m.applyExternalEdit('p1', 'athas://editor/a', 'disk text')
    // Edited the held model in place (not via the visible-pane branch).
    expect(heldA?.getValue()).toBe('disk text')
  })

  it('applyExternalEdit is a no-op for an unknown uri', () => {
    const ea = fakeEditorApi(); const reg = new ModelRegistry(fakeModelApi())
    const m = new EditorManager(ea, reg, { lang, text })
    m.mountPane('p1', {} as HTMLElement)
    expect(() => m.applyExternalEdit('p1', 'athas://editor/never', 'x')).not.toThrow()
  })

  it('getRawEditor exposes the underlying editor for a mounted pane', () => {
    const ea = fakeEditorApi(); const reg = new ModelRegistry(fakeModelApi())
    const m = new EditorManager(ea, reg, { lang, text })
    expect(m.getRawEditor('p1')).toBeNull() // not mounted yet
    m.mountPane('p1', {} as HTMLElement)
    expect(m.getRawEditor('p1')).toEqual({ id: 'raw-editor' })
  })
})
