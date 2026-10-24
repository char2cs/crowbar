import type { DocumentRef } from '@/lib/api/documents'
import { documentUri } from './document-ref'

interface Disposable {
  dispose(): void
}

/** The slice of a Monaco text model the registry and the editor manager use. */
export interface IModelLike {
  readonly uri: string
  dispose(): void
  getValue(): string
  getValueLength(): number
  getAlternativeVersionId(): number
  onDidChangeContent(listener: () => void): Disposable
  /** One undoable full replace; `preserveSelections` keeps every showing editor's carets. */
  replaceAll(text: string, preserveSelections: boolean): void
}

export interface MonacoModelApi {
  createModel(value: string, languageId: string, uri: string): IModelLike
  getModel(uri: string): IModelLike | null
}

export interface DocumentListener {
  dirtyChanged(uri: string, dirty: boolean): void
  contentChanged(uri: string): void
  opened(uri: string, ref: DocumentRef): void
  released(uri: string): void
}

interface OpenDocument {
  model: IModelLike
  savedText: string
  /** The model version that holds `savedText`; -1 when no version does. */
  savedVersionId: number
  dirty: boolean
  subscription: Disposable
}

type Listeners = Map<string, Set<() => void>>

function addTo(listeners: Listeners, uri: string, fn: () => void): () => void {
  const set = listeners.get(uri) ?? new Set()
  listeners.set(uri, set.add(fn))
  return () => {
    set.delete(fn)
    if (set.size === 0 && listeners.get(uri) === set) listeners.delete(uri)
  }
}

function callAll(listeners: Listeners, uri: string): void {
  for (const fn of [...(listeners.get(uri) ?? [])]) fn()
}

/**
 * The window's open documents: one Monaco model per document, created when it
 * opens and disposed when its buffer goes. The model owns the text; the
 * registry derives dirty from it and reports only the transitions.
 */
export class DocumentRegistry {
  private docs = new Map<string, OpenDocument>()
  private writers: Listeners = new Map()
  private externals: Listeners = new Map()
  private openListeners = new Set<(uri: string) => void>()

  constructor(
    private api: MonacoModelApi,
    private listener: DocumentListener,
  ) {}

  /** Idempotent per document. A model the references peek made is adopted with `text`. */
  open(ref: DocumentRef, languageId: string, text: string, savedText = text): IModelLike {
    const uri = documentUri(ref)
    const existing = this.docs.get(uri)
    if (existing) return existing.model
    const adopted = this.api.getModel(uri)
    if (adopted && adopted.getValue() !== text) adopted.replaceAll(text, false)
    const model = adopted ?? this.api.createModel(text, languageId, uri)
    const clean = text === savedText
    const doc: OpenDocument = {
      model,
      savedText,
      savedVersionId: clean ? model.getAlternativeVersionId() : -1,
      dirty: !clean,
      subscription: model.onDidChangeContent(() => this.changed(uri, doc)),
    }
    this.docs.set(uri, doc)
    this.listener.opened(uri, ref)
    for (const fn of [...this.openListeners]) fn(uri)
    return model
  }

  release(uri: string): void {
    const doc = this.docs.get(uri)
    if (!doc) return
    this.docs.delete(uri)
    doc.subscription.dispose()
    doc.model.dispose()
    this.listener.released(uri)
  }

  has(uri: string): boolean {
    return this.docs.has(uri)
  }

  get(uri: string): IModelLike | null {
    return this.docs.get(uri)?.model ?? null
  }

  /** The current text, after every pending writer of `uri` has landed its edits. */
  text(uri: string): string | null {
    callAll(this.writers, uri)
    return this.docs.get(uri)?.model.getValue() ?? null
  }

  savedText(uri: string): string | null {
    return this.docs.get(uri)?.savedText ?? null
  }

  isDirty(uri: string): boolean {
    return this.docs.get(uri)?.dirty ?? false
  }

  /** Record `text` as what disk holds; clean only if the model still holds exactly it. */
  markSaved(uri: string, text: string): void {
    const doc = this.docs.get(uri)
    if (!doc) return
    doc.savedText = text
    doc.savedVersionId = doc.model.getValue() === text ? doc.model.getAlternativeVersionId() : -1
    this.settleDirty(uri, doc)
  }

  /** A user edit: one undoable step. */
  replace(uri: string, text: string): void {
    const model = this.docs.get(uri)?.model
    if (model && model.getValue() !== text) model.replaceAll(text, false)
  }

  /** Text from outside the editor, undoable, leaving every editor's caret where it was. */
  applyExternal(uri: string, text: string, saved: boolean): void {
    const doc = this.docs.get(uri)
    if (!doc) return
    // Saved text is set first, so the change lands clean instead of flickering dirty.
    if (saved) doc.savedText = text
    if (doc.model.getValue() !== text) doc.model.replaceAll(text, true)
    if (saved) this.markSaved(uri, text)
    callAll(this.externals, uri)
  }

  registerPendingWriter(uri: string, flush: () => void): () => void {
    return addTo(this.writers, uri, flush)
  }

  onDidOpen(listener: (uri: string) => void): () => void {
    this.openListeners.add(listener)
    return () => {
      this.openListeners.delete(listener)
    }
  }

  onDidApplyExternal(uri: string, listener: () => void): () => void {
    return addTo(this.externals, uri, listener)
  }

  private changed(uri: string, doc: OpenDocument): void {
    this.settleDirty(uri, doc)
    this.listener.contentChanged(uri)
  }

  private settleDirty(uri: string, doc: OpenDocument): void {
    const { model, savedText } = doc
    const dirty =
      model.getAlternativeVersionId() !== doc.savedVersionId &&
      !(model.getValueLength() === savedText.length && model.getValue() === savedText)
    if (dirty === doc.dirty) return
    doc.dirty = dirty
    this.listener.dirtyChanged(uri, dirty)
  }
}
