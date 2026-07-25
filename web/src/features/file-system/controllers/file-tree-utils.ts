import type { AppFile } from '@/features/file-system/types/app'

/**
 * Depth-first lookup of the node at `path` in an explorer tree, or null when the
 * tree holds no such node. Paths are workspace-relative, matching the `path`
 * each node carries.
 *
 * This used to `return null` unconditionally. Both callers read that as a
 * truthful "not in the tree": the "Open All Files" fallback reported every
 * directory empty, and the inline create-file/folder duplicate-name guard never
 * fired. A lookup that cannot find anything is not a lookup.
 */
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
