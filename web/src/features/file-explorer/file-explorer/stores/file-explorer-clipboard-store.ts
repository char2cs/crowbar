import { create } from 'zustand'
import { copyFileNode, renameFileNode } from '@/features/files/lib/file-tree-api'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { getBaseName, joinPath } from '@/utils/path-helpers'
import { createSelectors } from '@/utils/zustand-selectors'

export interface ClipboardEntry {
  path: string
  is_dir: boolean
}

export interface FileClipboardState {
  entries: ClipboardEntry[]
  operation: 'copy' | 'cut'
}

export interface PastedEntry {
  source_path: string
  destination_path: string
  is_dir: boolean
  success: boolean
}

interface FileClipboardStore {
  clipboard: FileClipboardState | null
  actions: {
    copy: (entries: ClipboardEntry[]) => Promise<void>
    cut: (entries: ClipboardEntry[]) => Promise<void>
    paste: (targetDirectory: string) => Promise<PastedEntry[]>
    clear: () => Promise<void>
    setClipboard: (state: FileClipboardState | null) => void
  }
}

const useFileClipboardStoreBase = create<FileClipboardStore>()((set, get) => ({
  clipboard: null,
  actions: {
    copy: async (entries: ClipboardEntry[]) => {
      set({ clipboard: { entries, operation: 'copy' } })
    },
    cut: async (entries: ClipboardEntry[]) => {
      set({ clipboard: { entries, operation: 'cut' } })
    },
    /**
     * Complete the staged operation into `targetDirectory` (workspace-relative;
     * '' is the workspace root) and report one result per entry.
     *
     * This was a `return []` stub in crowbar-bridge behind a "FUTURE:" comment,
     * so Cmd+X then Cmd+V greyed a node out and then moved nothing, with no
     * error — a completed move the user was shown but never got.
     *
     * A cut is an atomic `rename`, never copy-then-delete: rename cannot leave
     * the tree half-moved, and the daemon refuses an occupied destination on
     * BOTH verbs (409), so a paste can never clobber an existing file.
     */
    paste: async (targetDirectory: string) => {
      const clipboard = get().clipboard
      if (!clipboard || clipboard.entries.length === 0) return []

      const wsId = getActiveWorkspaceId()
      if (!wsId) throw new Error('no active workspace for paste')

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

      // A cut entry is CONSUMED by the move that completed it — re-pasting it
      // would only 404 on a source that has moved. Entries whose move failed
      // stay staged so the user can retry after fixing the collision. A copy
      // stays staged in full: pasting a copy twice is a legitimate action.
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
    setClipboard: (state: FileClipboardState | null) => {
      set({ clipboard: state })
    },
  },
}))

export const useFileClipboardStore = createSelectors(useFileClipboardStoreBase)
