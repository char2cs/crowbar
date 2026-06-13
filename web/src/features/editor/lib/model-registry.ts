export interface IModelLike {
  uri: string
  dispose(): void
  /** Current full text of the model. */
  getValue(): string
  /** Replace the full text via an undo-friendly edit op IFF it differs from the
   *  current value. No-op (and pushes no undo stop) when text is unchanged. */
  setValueIfChanged(text: string): void
}
export interface MonacoModelApi {
  createModel(value: string, languageId: string, uri: string): IModelLike
  getModel(uri: string): IModelLike | null
}

interface Entry { model: IModelLike; refs: number }

/** Per-workspace registry of Monaco text models keyed by file URI.
 *  Same file in two panes => same model => live sync + shared undo. */
export class ModelRegistry {
  private entries = new Map<string, Entry>()
  constructor(private api: MonacoModelApi) {}

  acquire(uri: string, languageId: string, initialText: string): IModelLike {
    const existing = this.entries.get(uri)
    if (existing) { existing.refs++; return existing.model }
    const model = this.api.getModel(uri) ?? this.api.createModel(initialText, languageId, uri)
    this.entries.set(uri, { model, refs: 1 })
    return model
  }

  release(uri: string): void {
    const e = this.entries.get(uri)
    if (!e) return
    e.refs--
    if (e.refs <= 0) { this.entries.delete(uri); e.model.dispose() }
  }

  get(uri: string): IModelLike | null { return this.entries.get(uri)?.model ?? null }
  disposeAll(): void { for (const e of this.entries.values()) e.model.dispose(); this.entries.clear() }
}
