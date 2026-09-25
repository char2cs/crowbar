import { describe, expect, it, vi } from 'vitest'
import { ModelRegistry, type IModelLike } from '@/features/editor/lib/model-registry'

type FakeModel = IModelLike & { value: string; disposed: boolean }

function fakeApi() {
  const models = new Map<string, FakeModel>()
  return {
    models,
    createModel: vi.fn((value: string, _lang: string, uri: string) => {
      const m: FakeModel = {
        uri,
        value,
        disposed: false,
        dispose(this: { disposed: boolean }) {
          this.disposed = true
          models.delete(uri)
        },
        getValue() {
          return value
        },
        setValueIfChanged(next: string) {
          value = next
        },
      }
      models.set(uri, m)
      return m
    }),
    getModel: vi.fn((uri: string) => models.get(uri) ?? null),
  }
}

describe('ModelRegistry', () => {
  it("adopts a model created outside the registry with the buffer's text", () => {
    const api = fakeApi()
    const preview = api.createModel('on disk', 'ts', 'crowbar://editor/w/a.ts')
    api.createModel.mockClear()
    const r = new ModelRegistry(api)

    const m = r.acquire('crowbar://editor/w/a.ts', 'ts', 'unsaved buffer text')

    expect(m).toBe(preview)
    expect(m.getValue()).toBe('unsaved buffer text')
    expect(api.createModel).not.toHaveBeenCalled()
  })

  it('creates one model per uri and reuses it on re-acquire', () => {
    const api = fakeApi()
    const r = new ModelRegistry(api)
    const m1 = r.acquire('crowbar://editor/x', 'ts', 'a')
    const m2 = r.acquire('crowbar://editor/x', 'ts', 'a')
    expect(m1).toBe(m2)
    expect(api.createModel).toHaveBeenCalledTimes(1)
  })

  it('disposes the model only when the last holder releases', () => {
    const api = fakeApi()
    const r = new ModelRegistry(api)
    const m = r.acquire('crowbar://editor/x', 'ts', 'a') as unknown as { disposed: boolean }
    r.acquire('crowbar://editor/x', 'ts', 'a')
    r.release('crowbar://editor/x')
    expect(m.disposed).toBe(false)
    r.release('crowbar://editor/x')
    expect(m.disposed).toBe(true)
  })

  it('release of unknown uri is a no-op (no throw)', () => {
    const api = fakeApi()
    const r = new ModelRegistry(api)
    expect(() => r.release('crowbar://editor/none')).not.toThrow()
  })

  it('re-acquire after disposal creates a fresh model', () => {
    const api = fakeApi()
    const r = new ModelRegistry(api)
    r.acquire('crowbar://editor/x', 'ts', 'a')
    r.release('crowbar://editor/x')
    r.acquire('crowbar://editor/x', 'ts', 'b')
    expect(api.createModel).toHaveBeenCalledTimes(2)
  })
})
