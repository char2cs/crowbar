import { apiFetch } from '@/lib/api'
import type { AppFile } from '@/features/file-system/types/app'

export interface FileNodeDTO {
  name: string
  path: string
  type: 'file' | 'directory'
  children?: FileNodeDTO[]
  gitStatus?: string
}

// Map a backend file node onto the AppFile shape the explorer renders. A
// directory with no `children` key is "not yet loaded" — represented as
// `children: undefined` — which the lazy loader uses as its fetch sentinel; an
// empty loaded directory becomes `children: []`.
export function toAppFile(node: FileNodeDTO): AppFile {
  const isDir = node.type === 'directory'
  return {
    name: node.name,
    path: node.path,
    isDir,
    isDirectory: isDir,
    isFile: !isDir,
    children: node.children ? node.children.map(toAppFile) : undefined,
    gitStatus: node.gitStatus,
  }
}

// fetchFileTree loads a single directory level of the workspace tree. An absent
// `path` returns the workspace root; the backend tree is lazy (one level per
// call), so the explorer fetches deeper levels on expand.
export async function fetchFileTree(wsId: string, path?: string): Promise<AppFile[]> {
  const query = path ? `?path=${encodeURIComponent(path)}` : ''
  const nodes = await apiFetch<FileNodeDTO[]>(
    `/v0/workspaces/${encodeURIComponent(wsId)}/files/tree${query}`,
  )
  return nodes.map(toAppFile)
}

export function filesWsEndpoint(wsId: string): string {
  return `/v0/ws/files?wsId=${encodeURIComponent(wsId)}`
}

export function findNode(tree: AppFile[], path: string): AppFile | null {
  for (const node of tree) {
    if (node.path === path) return node
    if (node.children) {
      const found = findNode(node.children, path)
      if (found) return found
    }
  }
  return null
}

// mergeChildren returns a new tree with the directory at `parentPath` carrying
// `children`. Untouched branches keep their identity so React can skip them.
export function mergeChildren(tree: AppFile[], parentPath: string, children: AppFile[]): AppFile[] {
  return tree.map((node) => {
    if (node.path === parentPath) return { ...node, children }
    if (node.children) {
      return { ...node, children: mergeChildren(node.children, parentPath, children) }
    }
    return node
  })
}
