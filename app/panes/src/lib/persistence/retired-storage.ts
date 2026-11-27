import { removeLocal } from '@/lib/local-storage'

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

export async function retireOrphanedStorage(): Promise<void> {
  for (const key of RETIRED_LOCAL_STORAGE_KEYS) removeLocal(key)
  await Promise.all(RETIRED_DATABASES.map(deleteDatabase))
}

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
