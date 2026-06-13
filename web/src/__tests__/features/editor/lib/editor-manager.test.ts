import { describe, expect, it, vi } from 'vitest'
import { EditorManager } from '@/features/editor/lib/editor-manager'
import { ModelRegistry } from '@/features/editor/lib/model-registry'

function fakeModelApi() {
  const models = new Map<string, any>()
  return {
    createModel: vi.fn((v: string, _l: string, uri: string) => { const m = { uri, dispose: vi.fn(() => models.delete(uri)) }; models.set(uri, m); return m }),
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
})
