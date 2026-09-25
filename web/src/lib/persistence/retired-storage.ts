/**
 * Storage an earlier build wrote that nothing in this build reads: the retired
 * settings (editor engine, formatter, lint-on-save, external editor), the
 * scroll-debug overlay's switch, and the tree-sitter parser cache database.
 */
const RETIRED_LOCAL_STORAGE_KEYS = [
  'crowbar:settings:editorEngine',
  'crowbar:settings:formatter',
  'crowbar:settings:lintOnSave',
  'crowbar:settings:externalEditor',
  'crowbar:settings:customEditorCommand',
  'debug-scroll',
]
const RETIRED_DATABASES = ['crowbar-parser-cache']

/** Remove it; best-effort and idempotent, so it simply runs at every boot. */
export async function retireOrphanedStorage(): Promise<void> {
  for (const key of RETIRED_LOCAL_STORAGE_KEYS) {
    try {
      localStorage.removeItem(key)
    } catch {
      /* storage unavailable — nothing to remove */
    }
  }
  await Promise.all(RETIRED_DATABASES.map(deleteDatabase))
}

// Settles on `blocked` too: an older build holding it open deletes it on close.
function deleteDatabase(name: string): Promise<void> {
  return new Promise((resolve) => {
    try {
      const request = indexedDB.deleteDatabase(name)
      request.onsuccess = request.onerror = request.onblocked = () => resolve()
    } catch {
      resolve()
    }
  })
}
