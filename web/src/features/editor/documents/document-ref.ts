import { workspaceRef, type DocumentRef } from '@/lib/api/documents'
import { resolveAssetPath } from '@/features/editor/lib/asset-data-url'
import type { EditorContent } from '@/features/panes/types/pane-content'

const WORKSPACE_PREFIX = 'crowbar://editor/'
const HOST_PREFIX = 'crowbar://host/'

/**
 * The Monaco model uri of a document. Models are one global table, so the uri
 * names the authority and, for a workspace file, the workspace: two worktrees'
 * `src/a.ts` must never share a model.
 */
export function documentUri(ref: DocumentRef): string {
  if (ref.authority === 'host') return HOST_PREFIX + encodeURIComponent(ref.path)
  return WORKSPACE_PREFIX + encodeURIComponent(ref.workspaceId) + '/' + encodeURIComponent(ref.path)
}

/** The document a model uri names, as built or as Monaco re-encodes it; null for any other uri. */
export function refFromUri(uri: string): DocumentRef | null {
  if (uri.startsWith(HOST_PREFIX)) {
    const path = decodeURIComponent(uri.slice(HOST_PREFIX.length))
    return path ? { authority: 'host', path } : null
  }
  if (!uri.startsWith(WORKSPACE_PREFIX)) return null
  const rest = uri.slice(WORKSPACE_PREFIX.length)
  const slash = rest.indexOf('/')
  if (slash <= 0 || slash === rest.length - 1) return null
  return workspaceRef(
    decodeURIComponent(rest.slice(0, slash)),
    decodeURIComponent(rest.slice(slash + 1)),
  )
}

/** The document an editor buffer holds. */
export function documentRefOf(
  buffer: Pick<EditorContent, 'authority' | 'workspaceId' | 'path'>,
): DocumentRef {
  const path = buffer.path ?? ''
  return buffer.authority === 'host'
    ? { authority: 'host', path }
    : workspaceRef(buffer.workspaceId, path)
}

/**
 * `relativePath` resolved against the directory of `ref`, on the same
 * authority. A leading `/` is the filesystem root for a host document and the
 * worktree root for a workspace one; `?query` and `#fragment` are dropped.
 */
export function siblingRef(ref: DocumentRef, relativePath: string): DocumentRef {
  const slash = ref.path.lastIndexOf('/')
  const dir = slash === -1 ? '' : ref.path.slice(0, slash)
  const resolved = resolveAssetPath(dir, relativePath)
  return ref.authority === 'host'
    ? { authority: 'host', path: `/${resolved}` }
    : workspaceRef(ref.workspaceId, resolved)
}

export function sameDocument(a: DocumentRef, b: DocumentRef): boolean {
  if (a.authority === 'host' || b.authority === 'host') {
    return a.authority === b.authority && a.path === b.path
  }
  return a.workspaceId === b.workspaceId && a.path === b.path
}

/** The tab label of a document: its file name. */
export function documentName(ref: DocumentRef): string {
  return ref.path.split('/').pop() || ref.path
}
