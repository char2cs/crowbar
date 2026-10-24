import { getEditorHost } from '@/features/editor/host/editor-host'
import { bestEffort } from '@/lib/best-effort'
import { getDB } from '@/lib/persistence/idb'

/** The unsaved text of a dirty document, kept so a reload or a crash loses nothing. */
export interface EditorDraft {
  uri: string
  text: string
  savedText: string
  updatedAt: number
}

/** Documents edited since their draft was last written. */
const unwritten = new Set<string>()

/** The draft `uri` stands for now; null once its document is clean or closed. */
function draftOf(uri: string): EditorDraft | null {
  const documents = getEditorHost()?.documents
  if (!documents?.isDirty(uri)) return null
  const text = documents.text(uri)
  const savedText = documents.savedText(uri)
  if (text === null || savedText === null) return null
  return { uri, text, savedText, updatedAt: Date.now() }
}

function persist(uris: readonly string[]): Promise<void> {
  for (const uri of uris) unwritten.delete(uri)
  return putDrafts(uris.map(draftOf).filter((d): d is EditorDraft => d !== null))
}

/** A dirty document changed; its draft is written by `writeDraft` or `flushDrafts`. */
export function noteDraftChanged(uri: string): void {
  unwritten.add(uri)
}

/** Writes `uri`'s draft; the editor calls it once edits pause. */
export function writeDraft(uri: string): Promise<void> {
  return persist([uri])
}

export function clearDraft(uri: string): void {
  unwritten.delete(uri)
  bestEffort(
    getDB().then((db) => db.delete('editor-drafts', uri)),
    'clear draft',
  )
}

/** Writes every draft not written since its last edit. */
function flushDrafts(): Promise<void> {
  return persist([...unwritten])
}

export async function loadDrafts(): Promise<Map<string, EditorDraft>> {
  const db = await getDB()
  const drafts = await db.getAll('editor-drafts')
  return new Map(drafts.map((draft) => [draft.uri, draft]))
}

export async function putDrafts(drafts: readonly EditorDraft[]): Promise<void> {
  if (drafts.length === 0) return
  const db = await getDB()
  const tx = db.transaction('editor-drafts', 'readwrite')
  await Promise.all([...drafts.map((draft) => tx.store.put(draft)), tx.done])
}

// Edits may not have paused when the window goes away.
if (typeof window !== 'undefined') {
  window.addEventListener('pagehide', () => bestEffort(flushDrafts(), 'flush drafts'))
}
