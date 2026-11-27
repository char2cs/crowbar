import { apiFetch } from '@/lib/api/fetch'
import { filesBaseForWorkspace } from '@/lib/workspace-scope-url'
import type { AppFile } from '@/features/file-system/types/app'

interface FileNodeDTO {
  name: string
  path: string
  type: 'file' | 'directory'
  children?: FileNodeDTO[]
  gitStatus?: string
}

function toAppFile(node: FileNodeDTO): AppFile {
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

export async function fetchFileTree(wsId: string, path?: string): Promise<AppFile[]> {
  const query = path ? `?path=${encodeURIComponent(path)}` : ''
  const nodes = await apiFetch<FileNodeDTO[]>(`${filesBaseForWorkspace(wsId)}/tree${query}`)
  return nodes.map(toAppFile)
}

export async function createFileNode(
  wsId: string,
  path: string,
  type: 'file' | 'dir',
): Promise<void> {
  await apiFetch(filesBaseForWorkspace(wsId), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path, type }),
  })
}

export async function renameFileNode(wsId: string, path: string, newPath: string): Promise<void> {
  await apiFetch(filesBaseForWorkspace(wsId), {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path, newPath }),
  })
}

export async function deleteFileNode(wsId: string, path: string): Promise<void> {
  await apiFetch(filesBaseForWorkspace(wsId), {
    method: 'DELETE',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path }),
  })
}

export async function copyFileNode(
  wsId: string,
  sourcePath: string,
  destPath: string,
): Promise<void> {
  await apiFetch(`${filesBaseForWorkspace(wsId)}/copy`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ sourcePath, destPath }),
  })
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

export function mergeChildren(tree: AppFile[], parentPath: string, children: AppFile[]): AppFile[] {
  return tree.map((node) => {
    if (node.path === parentPath) return { ...node, children }
    if (node.children) {
      return { ...node, children: mergeChildren(node.children, parentPath, children) }
    }
    return node
  })
}
