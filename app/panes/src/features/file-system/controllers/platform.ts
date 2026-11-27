import { apiFetch } from '@/lib/api/fetch'
import { isNotFoundError } from '@/lib/errors/api-error'
import { filesBaseForWorkspace } from '@/lib/workspace-scope-url'

export interface DirectoryEntry {
  name: string
  path: string
  isDirectory: boolean
  is_dir: boolean
  isFile: boolean
}

interface FileTreeNode {
  name: string
  path: string
  type: 'file' | 'directory'
}

export async function readDirectory(workspaceId: string, path: string): Promise<DirectoryEntry[]> {
  const query = path ? `?path=${encodeURIComponent(path)}` : ''
  const nodes = await apiFetch<FileTreeNode[]>(`${filesBaseForWorkspace(workspaceId)}/tree${query}`)
  return nodes.map((node) => {
    const isDirectory = node.type === 'directory'
    return {
      name: node.name,
      path: node.path,
      isDirectory,
      is_dir: isDirectory,
      isFile: !isDirectory,
    }
  })
}

export async function exists(workspaceId: string, path: string): Promise<boolean> {
  const normalized = path.replace(/^\/+/, '').replace(/\/+$/, '')
  if (!normalized) return true

  const lastSlash = normalized.lastIndexOf('/')
  const parent = lastSlash === -1 ? '' : normalized.slice(0, lastSlash)

  try {
    const entries = await readDirectory(workspaceId, parent)
    return entries.some((entry) => entry.path === normalized)
  } catch (error) {
    if (isNotFoundError(error)) return false
    throw error
  }
}

export async function moveFile(workspaceId: string, src: string, dest: string): Promise<void> {
  await apiFetch(filesBaseForWorkspace(workspaceId), {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path: src, newPath: dest }),
  })
}
