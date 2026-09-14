import { create } from 'zustand'
import { combine } from 'zustand/middleware'
import { immer } from 'zustand/middleware/immer'
import type { FileEntry } from '@/features/file-system/types/app'

// Live-reported: "folders that were opened get lost when switching contexts."
// Root cause: expanded-folder state used to be a single flat Set shared by
// EVERY workspace — switching which workspace was active never touched it, so
// a freshly loaded workspace's tree looked at another workspace's leftover
// path strings (almost never a match: nothing appears expanded) and, going
// back, an intervening workspace's own toggles had silently mutated the same
// set. Keying it by workspace id fixes both directions at once. Every action
// below now takes the caller's wsId explicitly rather than reading an
// "active workspace" internally, so a background/kept-alive workspace's own
// effects (use-workspace-effects.ts) react to ITS OWN expansion, not
// whichever workspace the user happens to be looking at.
interface FileTreeState {
  expandedPathsByWorkspace: Record<string, Set<string>>
  selectedFiles: Set<string>
}

const EMPTY_EXPANDED_PATHS: ReadonlySet<string> = new Set()

function normalizeTreePath(path: string): string {
  return path.replace(/\\/g, '/').replace(/\/+$/, '')
}

function isPathWithinFolder(path: string, folderPath: string): boolean {
  const normalizedPath = normalizeTreePath(path)
  const normalizedFolderPath = normalizeTreePath(folderPath)

  return (
    normalizedPath === normalizedFolderPath || normalizedPath.startsWith(`${normalizedFolderPath}/`)
  )
}

export const useFileTreeStore = create(
  immer(
    combine(
      {
        expandedPathsByWorkspace: {} as Record<string, Set<string>>,
        selectedFiles: new Set<string>(),
      } as FileTreeState,
      (set, get) => ({
        toggleFolder: (wsId: string, path: string) => {
          set((state) => {
            const expanded = state.expandedPathsByWorkspace[wsId] ?? new Set<string>()
            if (expanded.has(path)) {
              expanded.delete(path)
            } else {
              expanded.add(path)
            }
            state.expandedPathsByWorkspace[wsId] = expanded
          })
        },

        selectFile: (path: string, multiSelect = false) => {
          set((state) => {
            if (multiSelect) {
              if (state.selectedFiles.has(path)) {
                state.selectedFiles.delete(path)
              } else {
                state.selectedFiles.add(path)
              }
            } else {
              state.selectedFiles.clear()
              state.selectedFiles.add(path)
            }
          })
        },

        clearSelection: () => {
          set((state) => {
            state.selectedFiles.clear()
          })
        },

        setExpandedPaths: (wsId: string, paths: Set<string>) => {
          set((state) => {
            state.expandedPathsByWorkspace[wsId] = paths
          })
        },

        getExpandedPaths: (wsId: string) => {
          return get().expandedPathsByWorkspace[wsId] ?? EMPTY_EXPANDED_PATHS
        },

        isExpanded: (wsId: string, path: string) => {
          return get().expandedPathsByWorkspace[wsId]?.has(path) ?? false
        },

        isSelected: (path: string) => {
          return get().selectedFiles.has(path)
        },

        expandToPath: (wsId: string, targetPath: string) => {
          set((state) => {
            const expanded = state.expandedPathsByWorkspace[wsId] ?? new Set<string>()
            const pathParts = targetPath.split(/[/\\]/)
            let currentPath = ''

            // Expand all parent folders leading to the target
            for (let i = 0; i < pathParts.length - 1; i++) {
              if (i === 0) {
                currentPath = pathParts[0]
              } else {
                currentPath += (targetPath.includes('\\') ? '\\' : '/') + pathParts[i]
              }
              expanded.add(currentPath)
            }
            state.expandedPathsByWorkspace[wsId] = expanded
          })
        },

        collapseAll: (wsId: string) => {
          set((state) => {
            state.expandedPathsByWorkspace[wsId] = new Set()
          })
        },

        collapsePath: (wsId: string, path: string) => {
          set((state) => {
            const expanded = state.expandedPathsByWorkspace[wsId]
            if (!expanded) return
            for (const expandedPath of Array.from(expanded)) {
              if (isPathWithinFolder(expandedPath, path)) {
                expanded.delete(expandedPath)
              }
            }
          })
        },

        expandAll: (wsId: string, files: FileEntry[]) => {
          set((state) => {
            const expanded = state.expandedPathsByWorkspace[wsId] ?? new Set<string>()
            const collectFolders = (items: FileEntry[]) => {
              for (const item of items) {
                if (item.isDir) {
                  expanded.add(item.path)
                  if (item.children) {
                    collectFolders(item.children)
                  }
                }
              }
            }
            collectFolders(files)
            state.expandedPathsByWorkspace[wsId] = expanded
          })
        },
      }),
    ),
  ),
)
