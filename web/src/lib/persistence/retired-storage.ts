import { removeLocal } from '@/lib/local-storage'

// Storage earlier builds wrote that nothing reads now: retired settings (editor engine, formatter,
// lint-on-save, external editor), the scroll-debug switch, and the tree-sitter parser cache.
// showBuildReadout is retired for buildReadout: boot stored every default, so its value is no choice.
const RETIRED_LOCAL_STORAGE_KEYS = [
  'crowbar:settings:editorEngine',
  'crowbar:settings:formatter',
  'crowbar:settings:lintOnSave',
  'crowbar:settings:externalEditor',
  'crowbar:settings:customEditorCommand',
  'debug-scroll',
  'crowbar:settings:showBuildReadout',
]
const RETIRED_DATABASES = ['crowbar-parser-cache']

/** Remove it; best-effort and idempotent, so it simply runs at every boot. */
export async function retireOrphanedStorage(): Promise<void> {
  for (const key of RETIRED_LOCAL_STORAGE_KEYS) removeLocal(key)
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
