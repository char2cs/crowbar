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

export function documentText(ref: DocumentRef): string | null {
  return documents()?.text(documentUri(ref)) ?? null
}

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

export function replaceDocumentText(ref: DocumentRef, text: string): void {
  documents()?.replace(documentUri(ref), text)
}

export function applyExternalText(ref: DocumentRef, text: string, opts: { saved: boolean }): void {
  const registry = documents()
  const uri = documentUri(ref)
  if (!registry?.has(uri)) return
  registry.applyExternal(uri, text, opts.saved)
  if (opts.saved) setDiskFlags(ref, { hasExternalChange: false, unavailable: undefined })
}

export function markDocumentSaved(ref: DocumentRef, text: string): void {
  const registry = documents()
  const uri = documentUri(ref)
  if (!registry?.has(uri)) return
  registry.markSaved(uri, text)
  if (!registry.isDirty(uri)) setDiskFlags(ref, { hasExternalChange: false })
}

export function isDocumentDirty(ref: DocumentRef): boolean {
  return documents()?.isDirty(documentUri(ref)) ?? false
}

export function documentSaveMark(ref: DocumentRef): number {
  return documents()?.saveMark(documentUri(ref)) ?? -1
}

export function savedDocumentText(ref: DocumentRef): string | null {
  return documents()?.savedText(documentUri(ref)) ?? null
}

export function onExternalText(ref: DocumentRef, listener: () => void): () => void {
  const uri = documentUri(ref)
  return withHost((host) => host.documents.onDidApplyExternal(uri, listener))
}

export function onDocumentOpen(ref: DocumentRef, listener: () => void): () => void {
  const uri = documentUri(ref)
  return withHost((host) =>
    host.documents.onDidOpen((opened) => {
      if (opened === uri) listener()
    }),
  )
}

export function registerPendingWriter(ref: DocumentRef, flush: () => void): () => void {
  const uri = documentUri(ref)
  return withHost((host) => host.documents.registerPendingWriter(uri, flush))
}
