import type { IModelLike, ModelRegistry } from '@/features/editor/lib/model-registry'

export interface IEditorLike {
  setModel(model: IModelLike | null): void
  getModel(): IModelLike | null
  saveViewState(): unknown
  restoreViewState(state: unknown): void
  layout(): void
  dispose(): void
  /** The underlying standalone editor (monaco `IStandaloneCodeEditor`), exposed
   *  as `unknown` so this module stays monaco-free. The React controller casts it
   *  to wire `updateOptions`/listeners. `null` when no raw editor backs the adapter
   *  (e.g. test fakes that don't model one). */
  raw(): unknown
}
export interface MonacoEditorApi {
  create(container: HTMLElement): IEditorLike
}
export interface BufferMeta {
  lang(uri: string): string
  text(uri: string): string
}

interface PaneState {
  editor: IEditorLike
  currentUri: string | null
  held: Set<string>
}

/** Owns one retained Monaco widget per pane. Tab switch = model swap (not remount);
 *  a model stays alive while its file is OPEN in the pane (held), not just visible. */
export class EditorManager {
  private panes = new Map<string, PaneState>()
  private viewState = new Map<string, unknown>() // key: `${paneId} ${uri}`
  constructor(
    private editorApi: MonacoEditorApi,
    private registry: ModelRegistry,
    private meta: BufferMeta,
  ) {}

  private vsKey(paneId: string, uri: string) {
    return `${paneId} ${uri}`
  }

  mountPane(paneId: string, container: HTMLElement): void {
    if (this.panes.has(paneId)) return
    this.panes.set(paneId, {
      editor: this.editorApi.create(container),
      currentUri: null,
      held: new Set(),
    })
  }

  showBuffer(paneId: string, uri: string): void {
    const pane = this.panes.get(paneId)
    if (!pane) return
    if (pane.currentUri === uri) return
    if (pane.currentUri) {
      this.viewState.set(this.vsKey(paneId, pane.currentUri), pane.editor.saveViewState())
    }
    let model: IModelLike
    if (pane.held.has(uri)) {
      model =
        this.registry.get(uri) ??
        this.registry.acquire(uri, this.meta.lang(uri), this.meta.text(uri))
    } else {
      model = this.registry.acquire(uri, this.meta.lang(uri), this.meta.text(uri))
      pane.held.add(uri)
    }
    pane.editor.setModel(model)
    const saved = this.viewState.get(this.vsKey(paneId, uri))
    if (saved) pane.editor.restoreViewState(saved)
    pane.currentUri = uri
  }

  closeBuffer(paneId: string, uri: string): void {
    const pane = this.panes.get(paneId)
    if (!pane || !pane.held.has(uri)) return
    this.registry.release(uri)
    pane.held.delete(uri)
    this.viewState.delete(this.vsKey(paneId, uri))
    if (pane.currentUri === uri) pane.currentUri = null
  }

  unmountPane(paneId: string): void {
    const pane = this.panes.get(paneId)
    if (!pane) return
    for (const uri of pane.held) this.registry.release(uri)
    pane.held.clear()
    pane.editor.dispose()
    this.panes.delete(paneId)
  }

  /** Apply a genuine EXTERNAL full-text replacement (disk reload, format-on-save)
   *  to the model for `uri`, preserving undo history far better than recreating
   *  the model. Targets the live model shown by the pane when it is displaying
   *  `uri`; otherwise edits the held registry model directly. No-op when the text
   *  is unchanged or the model is not held/known. */
  applyExternalEdit(paneId: string, uri: string, text: string): void {
    const pane = this.panes.get(paneId)
    if (pane && pane.currentUri === uri) {
      const model = pane.editor.getModel()
      if (model) {
        model.setValueIfChanged(text)
        return
      }
    }
    const model = this.registry.get(uri)
    model?.setValueIfChanged(text)
  }

  getEditor(paneId: string): IEditorLike | undefined {
    return this.panes.get(paneId)?.editor
  }
  /** The underlying monaco standalone editor for a pane, for the React controller
   *  to apply `updateOptions` and attach listeners. Cast at the call site. */
  getRawEditor(paneId: string): unknown {
    return this.panes.get(paneId)?.editor.raw() ?? null
  }
  layoutPane(paneId: string): void {
    this.panes.get(paneId)?.editor.layout()
  }
  /**
   * Lay out every mounted pane's editor. Used by staged reactivation (Task 39):
   * on a compositor-preserved reveal the ResizeObserver may not fire (the DOM
   * stayed laid out while hidden), so the host runs one deferred, post-first-frame
   * layout to correct any drift accrued while off-screen.
   */
  layoutAll(): void {
    for (const id of this.panes.keys()) this.layoutPane(id)
  }
  disposeAll(): void {
    for (const id of [...this.panes.keys()]) this.unmountPane(id)
    this.registry.disposeAll()
  }
}
