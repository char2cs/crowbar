/**
 * The one way to read or change a document's text outside the Monaco surface.
 * Backed by the buffer's `content` for now; callers never see where text lives.
 */
import type { DocumentRef } from '@/lib/api/documents'
import { setBufferContent } from '@/features/editor/lib/buffer-save'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import {
  isEditorContent,
  type EditorContent,
  type PaneContent,
} from '@/features/panes/types/pane-content'
import { documentRefOf, documentUri, sameDocument } from './document-ref'

const pendingWriters = new Map<string, Set<() => void>>()
const externalListeners = new Map<string, Set<() => void>>()

function holderIn(buffers: readonly PaneContent[], ref: DocumentRef): EditorContent | null {
  for (const b of buffers) {
    if (isEditorContent(b) && b.path && sameDocument(documentRefOf(b), ref)) return b
  }
  return null
}

function holderOf(ref: DocumentRef): EditorContent | null {
  return holderIn(windowPaneStore.getState().buffers, ref)
}

function patchHolder(ref: DocumentRef, patch: (b: EditorContent) => EditorContent): void {
  const id = holderOf(ref)?.id
  if (!id) return
  windowPaneStore.setState((state) => ({
    buffers: state.buffers.map((b) => (b.id === id && isEditorContent(b) ? patch(b) : b)),
  }))
}

function addTo(registry: Map<string, Set<() => void>>, ref: DocumentRef, fn: () => void) {
  const key = documentUri(ref)
  const set = registry.get(key) ?? new Set()
  registry.set(key, set.add(fn))
  return () => {
    set.delete(fn)
    if (set.size === 0 && registry.get(key) === set) registry.delete(key)
  }
}

function callAll(registry: Map<string, Set<() => void>>, ref: DocumentRef): void {
  for (const fn of [...(registry.get(documentUri(ref)) ?? [])]) fn()
}

/** Land edits an editor is still debouncing: Monaco's content sinks, then the ref's writers. */
function landPendingEdits(ref: DocumentRef): void {
  if (typeof window !== 'undefined') window.dispatchEvent(new Event('flush-editor-content'))
  callAll(pendingWriters, ref)
}

/** The document's current text, including edits a pending writer holds; null when not open. */
export function documentText(ref: DocumentRef): string | null {
  landPendingEdits(ref)
  return holderOf(ref)?.content ?? null
}

function documentOpened(ref: DocumentRef): Promise<void> {
  if (holderOf(ref)) return Promise.resolve()
  return new Promise((resolve) => {
    const stop = windowPaneStore.subscribe((state) => {
      if (!holderIn(state.buffers, ref)) return
      stop()
      resolve()
    })
  })
}

/** `documentText` once the document is open. */
export async function readDocumentText(ref: DocumentRef): Promise<string> {
  await documentOpened(ref)
  return documentText(ref) ?? readDocumentText(ref)
}

/** A user edit: undoable, and dirty unless it lands back on the saved text. */
export function replaceDocumentText(ref: DocumentRef, text: string): void {
  const holder = holderOf(ref)
  if (holder) setBufferContent(holder.id, text)
}

/** Text from outside the editor (a reload, a change on disk, a format result). */
export function applyExternalText(ref: DocumentRef, text: string, opts: { saved: boolean }): void {
  patchHolder(ref, (b) =>
    opts.saved
      ? {
          ...b,
          content: text,
          savedContent: text,
          isDirty: false,
          hasExternalChange: false,
          fileMissing: false,
        }
      : { ...b, content: text, isDirty: text !== b.savedContent },
  )
  callAll(externalListeners, ref)
}

/** Record `text` as what disk holds; the document is clean only if it still holds exactly that. */
export function markDocumentSaved(ref: DocumentRef, text: string): void {
  patchHolder(ref, (b) => ({
    ...b,
    savedContent: text,
    isDirty: b.content !== text,
    hasExternalChange: b.content === text ? false : b.hasExternalChange,
  }))
}

/** What disk held at the last load or save; null when not open. */
export function savedDocumentText(ref: DocumentRef): string | null {
  return holderOf(ref)?.savedContent ?? null
}

/** Called after each `applyExternalText` to `ref`, never for user edits. */
export function onExternalText(ref: DocumentRef, listener: () => void): () => void {
  return addTo(externalListeners, ref, listener)
}

/** `flush` writes edits an editor of `ref` is still holding; every read of the text runs it first. */
export function registerPendingWriter(ref: DocumentRef, flush: () => void): () => void {
  return addTo(pendingWriters, ref, flush)
}
