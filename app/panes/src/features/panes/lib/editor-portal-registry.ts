export interface EditorPortalEntry {
  node: HTMLDivElement
  activeEditorBufferId: string | null
  isActiveSurface: boolean
}

type Listener = (entry: EditorPortalEntry | undefined) => void

const entries = new Map<string, EditorPortalEntry>()
const listeners = new Map<string, Set<Listener>>()

function notify(paneId: string): void {
  const set = listeners.get(paneId)
  if (!set) return
  const entry = entries.get(paneId)
  for (const cb of set) cb(entry)
}

export function setEditorPortalEntry(paneId: string, entry: EditorPortalEntry): void {
  entries.set(paneId, entry)
  notify(paneId)
}

export function clearEditorPortalEntry(paneId: string): void {
  if (!entries.has(paneId)) return
  entries.delete(paneId)
  notify(paneId)
}

export function getEditorPortalEntry(paneId: string): EditorPortalEntry | undefined {
  return entries.get(paneId)
}

export function subscribeEditorPortalEntry(paneId: string, cb: Listener): () => void {
  let set = listeners.get(paneId)
  if (!set) {
    set = new Set()
    listeners.set(paneId, set)
  }
  set.add(cb)
  cb(entries.get(paneId))
  return () => {
    set!.delete(cb)
    if (set!.size === 0) listeners.delete(paneId)
  }
}
