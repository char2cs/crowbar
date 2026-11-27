import type { DocumentRegistry, IModelLike } from '@/features/editor/documents/document-registry'

export interface IEditorLike {
  setModel(model: IModelLike | null): void
  getModel(): IModelLike | null
  saveViewState(): unknown
  restoreViewState(state: unknown): void
  layout(): void
  dispose(): void
  raw(): unknown
}
export interface MonacoEditorApi {
  create(container: HTMLElement): IEditorLike
}

interface PaneState {
  editor: IEditorLike
  currentUri: string | null
}

export class EditorManager {
  private panes = new Map<string, PaneState>()
  private viewState = new Map<string, unknown>()
  constructor(
    private editorApi: MonacoEditorApi,
    private documents: Pick<DocumentRegistry, 'get'>,
  ) {}

  private vsKey(paneId: string, uri: string) {
    return `${paneId} ${uri}`
  }

  mountPane(paneId: string, container: HTMLElement): void {
    if (this.panes.has(paneId)) return
    this.panes.set(paneId, { editor: this.editorApi.create(container), currentUri: null })
  }

  showBuffer(paneId: string, uri: string): boolean {
    const pane = this.panes.get(paneId)
    if (!pane) return false
    if (pane.currentUri === uri && pane.editor.getModel()) return true
    if (pane.currentUri && pane.editor.getModel()) {
      this.viewState.set(this.vsKey(paneId, pane.currentUri), pane.editor.saveViewState())
    }
    const model: IModelLike | null = this.documents.get(uri)
    pane.editor.setModel(model)
    pane.currentUri = model ? uri : null
    if (!model) return false
    const saved = this.viewState.get(this.vsKey(paneId, uri))
    if (saved) pane.editor.restoreViewState(saved)
    return true
  }

  closeBuffer(paneId: string, uri: string): void {
    this.viewState.delete(this.vsKey(paneId, uri))
    const pane = this.panes.get(paneId)
    if (pane?.currentUri === uri) pane.currentUri = null
  }

  unmountPane(paneId: string): void {
    const pane = this.panes.get(paneId)
    if (!pane) return
    if (pane.currentUri && pane.editor.getModel()) {
      this.viewState.set(this.vsKey(paneId, pane.currentUri), pane.editor.saveViewState())
    }
    pane.editor.dispose()
    this.panes.delete(paneId)
  }

  getEditor(paneId: string): IEditorLike | undefined {
    return this.panes.get(paneId)?.editor
  }
  getRawEditor(paneId: string): unknown {
    return this.panes.get(paneId)?.editor.raw() ?? null
  }
  layoutPane(paneId: string): void {
    this.panes.get(paneId)?.editor.layout()
  }
}
