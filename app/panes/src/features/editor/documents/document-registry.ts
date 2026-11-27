import type { DocumentRef } from '@/lib/api/documents'
import { documentUri } from './document-ref'

interface Disposable {
  dispose(): void
}

export interface IModelLike {
  readonly uri: string
  dispose(): void
  getValue(): string
  getValueLength(): number
  getAlternativeVersionId(): number
  onDidChangeContent(listener: () => void): Disposable
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
  savedVersionId: number
  dirty: boolean
  saveMark: number
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

export class DocumentRegistry {
  private docs = new Map<string, OpenDocument>()
  private writers: Listeners = new Map()
  private externals: Listeners = new Map()
  private openListeners = new Set<(uri: string) => void>()
  private saves = 0

  constructor(
    private api: MonacoModelApi,
    private listener: DocumentListener,
  ) {}

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
      saveMark: ++this.saves,
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
    this.listener.released(uri)
    doc.model.dispose()
  }

  has(uri: string): boolean {
    return this.docs.has(uri)
  }

  get(uri: string): IModelLike | null {
    return this.docs.get(uri)?.model ?? null
  }

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

  saveMark(uri: string): number {
    return this.docs.get(uri)?.saveMark ?? -1
  }

  markSaved(uri: string, text: string): void {
    const doc = this.docs.get(uri)
    if (!doc) return
    doc.savedText = text
    doc.savedVersionId = doc.model.getValue() === text ? doc.model.getAlternativeVersionId() : -1
    doc.saveMark = ++this.saves
    this.settleDirty(uri, doc)
  }

  replace(uri: string, text: string): void {
    const model = this.docs.get(uri)?.model
    if (model && model.getValue() !== text) model.replaceAll(text, false)
  }

  applyExternal(uri: string, text: string, saved: boolean): void {
    const doc = this.docs.get(uri)
    if (!doc) return
    if (doc.model.getValue() === text && (!saved || doc.savedText === text)) return
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
