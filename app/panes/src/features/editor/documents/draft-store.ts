import { getEditorHost } from '@/features/editor/host/editor-host'
import { report } from '@/lib/errors/report'
import { commit, getDB } from '@/lib/persistence/idb'

export interface EditorDraft {
  uri: string
  text: string
  savedText: string
  updatedAt: number
}

const unwritten = new Set<string>()

function draftOf(uri: string): EditorDraft | null {
  const documents = getEditorHost()?.documents
  if (!documents?.isDirty(uri)) return null
  const text = documents.text(uri)
  const savedText = documents.savedText(uri)
  if (text === null || savedText === null) return null
  return { uri, text, savedText, updatedAt: Date.now() }
}

function persist(uris: readonly string[]): void {
  for (const uri of uris) unwritten.delete(uri)
  const drafts = uris.map(draftOf).filter((d): d is EditorDraft => d !== null)
  void putDrafts(drafts).catch((err: unknown) => report(err, { action: 'Keep the unsaved draft' }))
}

export function noteDraftChanged(uri: string): void {
  unwritten.add(uri)
}

export function writeDraft(uri: string): void {
  persist([uri])
}

export function clearDraft(uri: string): void {
  unwritten.delete(uri)
  void getDB()
    .then((db) => db.delete('editor-drafts', uri))
    .catch((err: unknown) => report(err, { action: 'Discard the saved draft' }))
}

function flushDrafts(): void {
  persist([...unwritten])
}

export async function loadDrafts(): Promise<Map<string, EditorDraft>> {
  const db = await getDB()
  const drafts = await db.getAll('editor-drafts')
  return new Map(drafts.map((draft) => [draft.uri, draft]))
}

export async function loadDraft(uri: string): Promise<EditorDraft | undefined> {
  return (await getDB()).get('editor-drafts', uri)
}

export async function putDrafts(drafts: readonly EditorDraft[]): Promise<void> {
  if (drafts.length === 0) return
  const db = await getDB()
  const tx = db.transaction('editor-drafts', 'readwrite')
  await commit(tx, () => Promise.all(drafts.map((draft) => tx.store.put(draft))))
}

if (typeof window !== 'undefined') {
  window.addEventListener('pagehide', flushDrafts)
}
