import { readDirectory } from '@/features/file-system/controllers/platform'
import type { FileEntry } from '@/features/file-system/types/app'

/**
 * The workspace-relative path a breadcrumb segment addresses.
 *
 * Deliberately NOT joined onto `rootFolderPath`: that field holds the WORKSPACE
 * ID (use-workspace-effects sets `rootFolderPath: wsId`), not a filesystem
 * prefix, so joining it produced `<wsId>/web/src` — a path the daemon's
 * safepath.Resolve cannot find, which left the segment dropdown permanently
 * empty even once `readDirectory` stopped stubbing. The breadcrumb's segments
 * are already workspace-relative, so the join is the whole answer.
 */
export function breadcrumbSegmentPath(segments: string[], index: number): string {
  return segments.slice(0, index + 1).join('/')
}

/**
 * List `path` for the breadcrumb's segment dropdown: directories first, then
 * case-insensitive by name. Rejects on failure so the caller can report it
 * rather than showing an empty popup.
 */
export async function loadDirectoryEntries(path: string): Promise<FileEntry[]> {
  const entries = await readDirectory(path)

  const fileEntries: FileEntry[] = entries.map((entry) => ({
    name: entry.name || 'Unknown',
    path: entry.path,
    isDir: entry.is_dir,
    children: undefined,
  }))

  fileEntries.sort((a, b) => {
    if (a.isDir && !b.isDir) return -1
    if (!a.isDir && b.isDir) return 1
    return a.name.toLowerCase().localeCompare(b.name.toLowerCase())
  })

  return fileEntries
}
