import { create } from 'zustand'
import { copyFileNode, renameFileNode } from '@/features/files/lib/file-tree-api'
import { getBaseName, joinPath } from '@/utils/path-helpers'
import { createSelectors } from '@/utils/zustand-selectors'
import { UserFacingError } from '@/lib/errors/user-facing-error'

export interface ClipboardEntry {
  path: string
  is_dir: boolean
}

interface FileClipboardState {
  entries: ClipboardEntry[]
  operation: 'copy' | 'cut'
  /** The workspace the entry paths are relative to: they name other files anywhere else. */
  wsId: string | null
}

interface PastedEntry {
  source_path: string
  destination_path: string
  is_dir: boolean
  success: boolean
}

interface FileClipboardStore {
  clipboard: FileClipboardState | null
  actions: {
    copy: (entries: ClipboardEntry[], wsId: string | null) => Promise<void>
    cut: (entries: ClipboardEntry[], wsId: string | null) => Promise<void>
    paste: (targetDirectory: string, wsId: string | null) => Promise<PastedEntry[]>
    clear: () => Promise<void>
  }
}

const useFileClipboardStoreBase = create<FileClipboardStore>()((set, get) => ({
  clipboard: null,
  actions: {
    copy: async (entries: ClipboardEntry[], wsId: string | null) => {
      set({ clipboard: { entries, operation: 'copy', wsId } })
    },
    cut: async (entries: ClipboardEntry[], wsId: string | null) => {
      set({ clipboard: { entries, operation: 'cut', wsId } })
    },
    /** Complete the staged operation into `targetDirectory` ('' is the workspace root), one
     *  result per entry. A cut is an atomic `rename`, never copy-then-delete, so nothing is
     *  left half-moved; the daemon 409s an occupied destination on both verbs: no clobbering. */
    paste: async (targetDirectory: string, wsId: string | null) => {
      const clipboard = get().clipboard
      if (!clipboard || clipboard.entries.length === 0) return []

      if (!wsId) throw new Error('no workspace to paste into')
      if (clipboard.wsId !== wsId) {
        throw new UserFacingError('Copied from another workspace — paste there, or copy again here')
      }

      const results: PastedEntry[] = []
      for (const entry of clipboard.entries) {
        const name = getBaseName(entry.path)
        const destination = targetDirectory ? joinPath(targetDirectory, name) : name
        try {
          // react-doctor-disable-next-line async-await-in-loop -- kept sequential: the daemon serialises FS mutations and emits one structural FileChangeEvent per node; a parallel burst would race the tree reconciliation. Cold path (manual paste), never hot.
          await (clipboard.operation === 'cut'
            ? renameFileNode(wsId, entry.path, destination)
            : copyFileNode(wsId, entry.path, destination))
          results.push({
            source_path: entry.path,
            destination_path: destination,
            is_dir: entry.is_dir,
            success: true,
          })
        } catch {
          results.push({
            source_path: entry.path,
            destination_path: destination,
            is_dir: entry.is_dir,
            success: false,
          })
        }
      }

      // A completed cut entry is consumed (re-pasting would 404 on the moved source); failed ones
      // stay staged for a retry. A copy stays staged in full: pasting it twice is legitimate.
      if (clipboard.operation === 'cut') {
        const failed = new Set<string>()
        for (const r of results) if (!r.success) failed.add(r.source_path)
        const remaining = clipboard.entries.filter((entry) => failed.has(entry.path))
        set({ clipboard: remaining.length > 0 ? { ...clipboard, entries: remaining } : null })
      }

      return results
    },
    clear: async () => {
      set({ clipboard: null })
    },
  },
}))

export const useFileClipboardStore = createSelectors(useFileClipboardStoreBase)
