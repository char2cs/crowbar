import { fetchFileTree } from '@/features/files/lib/file-tree-api'
import type { FileEntry } from '@/features/file-system/types/app'

/**
 * The workspace-relative path a breadcrumb segment addresses. Segments are
 * already workspace-relative, so the join is the whole answer.
 */
export function breadcrumbSegmentPath(segments: string[], index: number): string {
  return segments.slice(0, index + 1).join('/')
}

/**
 * List `path` of `workspaceId` for the breadcrumb's segment dropdown:
 * directories first, then case-insensitive by name. Rejects on failure so the
 * caller can report it rather than showing an empty popup.
 */
export async function loadDirectoryEntries(
  workspaceId: string,
  path: string,
): Promise<FileEntry[]> {
  const entries = await fetchFileTree(workspaceId, path)

  const fileEntries: FileEntry[] = entries.map((entry) => ({
    name: entry.name || 'Unknown',
    path: entry.path,
    isDir: entry.isDir,
    children: undefined,
  }))

  fileEntries.sort((a, b) => {
    if (a.isDir && !b.isDir) return -1
    if (!a.isDir && b.isDir) return 1
    return a.name.toLowerCase().localeCompare(b.name.toLowerCase())
  })

  return fileEntries
}
