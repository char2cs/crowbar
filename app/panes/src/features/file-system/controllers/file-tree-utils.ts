import type { AppFile } from '@/features/file-system/types/app'

/** Depth-first lookup of the node at `path` (workspace-relative, as nodes carry it), or null.
 *  Callers trust null as "not in the tree": the Open All Files fallback and the inline create
 *  duplicate-name guard both act on it. */
export function findFileInTree(tree: AppFile[], path: string): AppFile | null {
  for (const node of tree) {
    if (node.path === path) return node
    if (node.children) {
      const found = findFileInTree(node.children, path)
      if (found) return found
    }
  }
  return null
}
