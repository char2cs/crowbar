/**
 * The one way to read or change a document's text outside the Monaco surface.
 * The text lives in the document's Monaco model; callers never see where.
 */
import type { DocumentRef } from '@/lib/api/documents'
import {
  getEditorHost,
  loadEditorHost,
  subscribeEditorHost,
  type EditorHost,
} from '@/features/editor/host/editor-host'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { isEditorContent, type EditorContent } from '@/features/panes/types/pane-content'
import { documentRefOf, documentUri, sameDocument } from './document-ref'

type DiskFlags = Partial<Pick<EditorContent, 'hasExternalChange' | 'unavailable'>>

const documents = () => getEditorHost()?.documents ?? null

/** Sets the holder's disk flags, writing only when one actually changes. */
function setDiskFlags(ref: DocumentRef, flags: DiskFlags): void {
  const holder = windowPaneStore
    .getState()
    .buffers.find((b) => isEditorContent(b) && b.path && sameDocument(documentRefOf(b), ref))
  if (!holder || !isEditorContent(holder)) return
  const keys = Object.keys(flags) as (keyof DiskFlags)[]
  if (keys.every((key) => Boolean(holder[key]) === Boolean(flags[key]))) return
  windowPaneStore.setState((state) => ({
    buffers: state.buffers.map((b) => (b.id === holder.id ? { ...b, ...flags } : b)),
  }))
}

/** Runs `attach` against the host now, or once it has loaded. */
function withHost(attach: (host: EditorHost) => () => void): () => void {
  const host = getEditorHost()
  if (host) return attach(host)
  let detach: (() => void) | null = null
  const stop = subscribeEditorHost(() => {
    const loaded = getEditorHost()
    if (loaded) detach = attach(loaded)
  })
  return () => {
    stop()
    detach?.()
  }
}

/** The document's current text, including edits a pending writer holds; null when not open. */
export function documentText(ref: DocumentRef): string | null {
  return documents()?.text(documentUri(ref)) ?? null
}

/** `documentText` once the document is open. */
export async function readDocumentText(ref: DocumentRef): Promise<string> {
  const host = await loadEditorHost()
  const uri = documentUri(ref)
  if (!host.documents.has(uri)) {
    await new Promise<void>((resolve) => {
      const stop = host.documents.onDidOpen((opened) => {
        if (opened !== uri) return
        stop()
        resolve()
      })
    })
  }
  return host.documents.text(uri) ?? readDocumentText(ref)
}

/** A user edit: undoable, and dirty unless it lands back on the saved text. */
export function replaceDocumentText(ref: DocumentRef, text: string): void {
  documents()?.replace(documentUri(ref), text)
}

/** Text from outside the editor (a reload, a change on disk, a format result). */
export function applyExternalText(ref: DocumentRef, text: string, opts: { saved: boolean }): void {
  const registry = documents()
  const uri = documentUri(ref)
  if (!registry?.has(uri)) return
  registry.applyExternal(uri, text, opts.saved)
  if (opts.saved) setDiskFlags(ref, { hasExternalChange: false, unavailable: undefined })
}

/** Record `text` as what disk holds; the document is clean only if it still holds exactly that. */
export function markDocumentSaved(ref: DocumentRef, text: string): void {
  const registry = documents()
  const uri = documentUri(ref)
  if (!registry?.has(uri)) return
  registry.markSaved(uri, text)
  if (!registry.isDirty(uri)) setDiskFlags(ref, { hasExternalChange: false })
}

/** Whether the document holds edits disk does not; false when not open. */
export function isDocumentDirty(ref: DocumentRef): boolean {
  return documents()?.isDirty(documentUri(ref)) ?? false
}

/** Moves on with each save and each open, so a read begun before the latest one is told apart. */
export function documentSaveMark(ref: DocumentRef): number {
  return documents()?.saveMark(documentUri(ref)) ?? -1
}

/** What disk held at the last load or save; null when not open. */
export function savedDocumentText(ref: DocumentRef): string | null {
  return documents()?.savedText(documentUri(ref)) ?? null
}

/** Called after each `applyExternalText` to `ref`, never for user edits. */
export function onExternalText(ref: DocumentRef, listener: () => void): () => void {
  const uri = documentUri(ref)
  return withHost((host) => host.documents.onDidApplyExternal(uri, listener))
}

/** Called whenever `ref`'s document opens, from now on. */
export function onDocumentOpen(ref: DocumentRef, listener: () => void): () => void {
  const uri = documentUri(ref)
  return withHost((host) =>
    host.documents.onDidOpen((opened) => {
      if (opened === uri) listener()
    }),
  )
}

/** `flush` writes edits an editor of `ref` is still holding; every read of the text runs it first. */
export function registerPendingWriter(ref: DocumentRef, flush: () => void): () => void {
  const uri = documentUri(ref)
  return withHost((host) => host.documents.registerPendingWriter(uri, flush))
}
