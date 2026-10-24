import { describe, expect, it, vi } from 'vitest'
import type { IModelLike } from '@/features/editor/documents/document-registry'
import {
  EditorManager,
  type IEditorLike,
  type MonacoEditorApi,
} from '@/features/editor/lib/editor-manager'

/** The open documents' models, as the registry hands them out. */
function fakeDocuments(...uris: string[]) {
  const models = new Map(
    uris.map((uri) => [uri, { uri, dispose: vi.fn() } as unknown as IModelLike]),
  )
  return { get: (uri: string) => models.get(uri) ?? null }
}

function fakeEditorApi(): MonacoEditorApi & { created: IEditorLike[] } {
  const created: IEditorLike[] = []
  return {
    created,
    create: vi.fn(() => {
      let model: IModelLike | null = null
      const ed: IEditorLike = {
        setModel: vi.fn((m: IModelLike | null) => {
          model = m
        }),
        getModel: () => model,
        saveViewState: vi.fn(() => ({ for: model?.uri })),
        restoreViewState: vi.fn(),
        layout: vi.fn(),
        dispose: vi.fn(),
        raw: vi.fn(() => ({ id: 'raw-editor' })),
      }
      created.push(ed)
      return ed
    }),
  }
}

const A = 'crowbar://editor/a'
const B = 'crowbar://editor/b'

function setup(...panes: string[]) {
  const ea = fakeEditorApi()
  const documents = fakeDocuments(A, B)
  const m = new EditorManager(ea, documents)
  for (const pane of panes) m.mountPane(pane, {} as HTMLElement)
  return { ea, documents, m }
}

describe('EditorManager', () => {
  it('creates exactly one widget per pane', () => {
    const { ea, m } = setup('p1')
    m.mountPane('p1', {} as HTMLElement)
    expect(ea.create).toHaveBeenCalledTimes(1)
  })

  it('showBuffer swaps to the open document’s model without a new widget', () => {
    const { ea, documents, m } = setup('p1')
    expect(m.showBuffer('p1', A)).toBe(true)
    expect(m.showBuffer('p1', B)).toBe(true)
    expect(ea.create).toHaveBeenCalledTimes(1)
    expect(ea.created[0].getModel()).toBe(documents.get(B))
  })

  it('a document that is not open leaves the pane empty and answers false', () => {
    const { ea, m } = setup('p1')
    m.showBuffer('p1', A)
    expect(m.showBuffer('p1', 'crowbar://editor/closed')).toBe(false)
    expect(ea.created[0].getModel()).toBeNull()
  })

  it('round-trips view-state: state saved on leaving a document is restored on returning', () => {
    const { ea, m } = setup('p1')
    m.showBuffer('p1', A)
    const ed = ea.created[0]
    const stateA = { scroll: 42 }
    ed.saveViewState = () => stateA
    m.showBuffer('p1', B)
    m.showBuffer('p1', A)
    expect(ed.restoreViewState).toHaveBeenCalledWith(stateA)
  })

  it('two panes on one document show its one model', () => {
    const { ea, m } = setup('p1', 'p2')
    m.showBuffer('p1', A)
    m.showBuffer('p2', A)
    expect(ea.created[0].getModel()).toBe(ea.created[1].getModel())
  })

  it('closeBuffer forgets where the pane was in that document', () => {
    const { ea, m } = setup('p1')
    m.showBuffer('p1', A)
    m.showBuffer('p1', B)
    m.closeBuffer('p1', A)
    m.showBuffer('p1', A)
    expect(ea.created[0].restoreViewState).not.toHaveBeenCalled()
  })

  it('unmountPane disposes the widget and leaves the document’s model alone', () => {
    const { ea, documents, m } = setup('p1')
    m.showBuffer('p1', A)
    m.unmountPane('p1')
    expect(ea.created[0].dispose).toHaveBeenCalled()
    expect(documents.get(A)!.dispose).not.toHaveBeenCalled()
  })

  it('getRawEditor exposes the underlying editor for a mounted pane', () => {
    const { m } = setup()
    expect(m.getRawEditor('p1')).toBeNull()
    m.mountPane('p1', {} as HTMLElement)
    expect(m.getRawEditor('p1')).toEqual({ id: 'raw-editor' })
  })
})
