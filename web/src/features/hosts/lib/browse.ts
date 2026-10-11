import { apiFetch } from '@/lib/api'

/** One child of a directory on a host's own filesystem (`GET /v0/system/browse`). */
export interface BrowseEntry {
  name: string
  /** Absolute, under the listed directory: a symlink keeps its own path, never its target's. */
  path: string
  /** For a symlink, whether its target is a directory. */
  isDir: boolean
}

/** Lists `path` on `hostId`'s filesystem, folders first. A refusal rejects with the
 *  daemon's or the proxy's `ApiError` unchanged, so the caller can say why. */
export function browseDirectory(hostId: string, path: string): Promise<BrowseEntry[]> {
  return apiFetch<BrowseEntry[]>(
    `/v0/system/browse?path=${encodeURIComponent(path)}`,
    undefined,
    undefined,
    hostId,
  )
}
