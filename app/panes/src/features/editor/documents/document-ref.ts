import { workspaceRef, type DocumentRef } from '@/lib/api/documents'
import { resolveAssetPath } from '@/features/editor/lib/asset-data-url'
import type { EditorContent } from '@/features/panes/types/pane-content'

const WORKSPACE_PREFIX = 'crowbar://editor/'
const HOST_PREFIX = 'crowbar://host/'

export function documentUri(ref: DocumentRef): string {
  if (ref.authority === 'host') return HOST_PREFIX + encodeURIComponent(ref.path)
  return WORKSPACE_PREFIX + encodeURIComponent(ref.workspaceId) + '/' + encodeURIComponent(ref.path)
}

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

export function documentRefOf(
  buffer: Pick<EditorContent, 'authority' | 'workspaceId' | 'path'>,
): DocumentRef {
  const path = buffer.path ?? ''
  return buffer.authority === 'host'
    ? { authority: 'host', path }
    : workspaceRef(buffer.workspaceId, path)
}

export function siblingRef(ref: DocumentRef, relativePath: string): DocumentRef {
  const slash = ref.path.lastIndexOf('/')
  const dir = slash === -1 ? '' : ref.path.slice(0, slash)
  const resolved = resolveAssetPath(dir, relativePath)
  return ref.authority === 'host'
    ? { authority: 'host', path: `/${resolved}` }
    : workspaceRef(ref.workspaceId, resolved)
}

export function isUntitledRef(ref: Pick<DocumentRef, 'path'>): boolean {
  return ref.path.startsWith('untitled:')
}

export function sameDocument(a: DocumentRef, b: DocumentRef): boolean {
  if (a.authority === 'host' || b.authority === 'host') {
    return a.authority === b.authority && a.path === b.path
  }
  return a.workspaceId === b.workspaceId && a.path === b.path
}

export function documentName(ref: DocumentRef): string {
  return ref.path.split('/').pop() || ref.path
}
