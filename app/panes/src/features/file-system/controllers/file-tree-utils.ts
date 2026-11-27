import type { AppFile } from '@/features/file-system/types/app'

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
