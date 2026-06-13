import type { IModelLike, ModelRegistry } from '@/features/editor/lib/model-registry'

export interface IEditorLike {
  setModel(model: IModelLike | null): void
  getModel(): IModelLike | null
  saveViewState(): unknown
  restoreViewState(state: unknown): void
  layout(): void
  dispose(): void
}
export interface MonacoEditorApi { create(container: HTMLElement): IEditorLike }
export interface BufferMeta { lang(uri: string): string; text(uri: string): string }

interface PaneState { editor: IEditorLike; currentUri: string | null }

/** Owns one retained Monaco widget per pane. Tab switch = model swap, not remount. */
export class EditorManager {
  private panes = new Map<string, PaneState>()
  private viewState = new Map<string, unknown>() // key: `${paneId} ${uri}`
  constructor(private editorApi: MonacoEditorApi, private registry: ModelRegistry, private meta: BufferMeta) {}

  private vsKey(paneId: string, uri: string) { return `${paneId} ${uri}` }

  mountPane(paneId: string, container: HTMLElement): void {
    if (this.panes.has(paneId)) return
    this.panes.set(paneId, { editor: this.editorApi.create(container), currentUri: null })
  }

  showBuffer(paneId: string, uri: string): void {
    const pane = this.panes.get(paneId)
    if (!pane) return
    if (pane.currentUri === uri) return
    if (pane.currentUri) {
      this.viewState.set(this.vsKey(paneId, pane.currentUri), pane.editor.saveViewState())
      this.registry.release(pane.currentUri)
    }
    const model = this.registry.acquire(uri, this.meta.lang(uri), this.meta.text(uri))
    pane.editor.setModel(model)
    const saved = this.viewState.get(this.vsKey(paneId, uri))
    if (saved) pane.editor.restoreViewState(saved)
    pane.currentUri = uri
  }

  unmountPane(paneId: string): void {
    const pane = this.panes.get(paneId)
    if (!pane) return
    if (pane.currentUri) this.registry.release(pane.currentUri)
    pane.editor.dispose()
    this.panes.delete(paneId)
  }

  getEditor(paneId: string): IEditorLike | undefined { return this.panes.get(paneId)?.editor }
  layoutPane(paneId: string): void { this.panes.get(paneId)?.editor.layout() }
  disposeAll(): void { for (const id of [...this.panes.keys()]) this.unmountPane(id); this.registry.disposeAll() }
}
