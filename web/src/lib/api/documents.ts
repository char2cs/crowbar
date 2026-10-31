import { apiFetch } from '@/lib/api/fetch'
import { problemOf, type Problem, type ProblemCode } from '@/lib/errors/problem'
import { report } from '@/lib/errors/report'
import {
  decodeFileContent,
  type FileContentPayload,
} from '@/features/file-system/utils/file-content-encoding'

/** One file: a path relative to a workspace's worktree, or a host file outside every worktree. */
export type DocumentRef =
  | { authority: 'workspace'; workspaceId: string; path: string }
  | { authority: 'host'; path: string }

/** The ref of `path`, relative to the worktree of `workspaceId`. */
export function workspaceRef(workspaceId: string, path: string): DocumentRef {
  return { authority: 'workspace', workspaceId, path }
}

/** What a read can name: a ref, or an absolute path the daemon resolves to one. */
export type DocumentTarget = DocumentRef | { absolutePath: string }

export interface DocumentRead {
  ref: DocumentRef
  text: string
}

/** The daemon's refusal of a host document asked for off the local socket. */
export const HOST_DOCUMENT_TRANSPORT_CODE = 'host_document_requires_local_transport'

/** The daemon's refusal of a file under its own state directory. */
export const DAEMON_STATE_DOCUMENT_CODE = 'daemon_state_document'

const DOCUMENTS = '/v0/documents/content'

function toRef(wire: DocumentRef & { workspaceId?: string }): DocumentRef {
  return wire.authority === 'workspace'
    ? { authority: 'workspace', workspaceId: wire.workspaceId ?? '', path: wire.path }
    : { authority: 'host', path: wire.path }
}

function query(target: DocumentTarget): string {
  if ('absolutePath' in target) return `path=${encodeURIComponent(target.absolutePath)}`
  if (target.authority === 'host') return `path=${encodeURIComponent(target.path)}`
  return `workspace=${encodeURIComponent(target.workspaceId)}&path=${encodeURIComponent(target.path)}`
}

/** Read a document; the answer carries the ref the daemon resolved the target to. */
export async function readDocument(target: DocumentTarget): Promise<DocumentRead> {
  const doc = await apiFetch<DocumentRef & FileContentPayload>(`${DOCUMENTS}?${query(target)}`)
  return { ref: toRef(doc), text: decodeFileContent(doc) }
}

/** The daemon would not arm a host document's watch, and why. */
export class WatchRefusal extends Error {
  constructor(readonly problem: Problem) {
    super(problem.message)
    this.name = 'WatchRefusal'
  }
}

/** The daemon refused a host document's watch because the OS has no file watches left. */
export function isWatchLimit(refusal: unknown): boolean {
  return refusal instanceof WatchRefusal && refusal.problem.code === 'watch_limit'
}

// A peek may point at a host or SDK file the transport or the daemon will not serve.
export const PREVIEW_ABSENT: readonly ProblemCode[] = [
  'not_found',
  'forbidden',
  HOST_DOCUMENT_TRANSPORT_CODE,
]

/** The document's text as saved on disk; null when it is `absent` or the read failed (reported). */
export async function readDocumentFromDisk(
  target: DocumentTarget,
  absent: readonly ProblemCode[] = ['not_found'],
): Promise<string | null> {
  try {
    return (await readDocument(target)).text
  } catch (err) {
    if (!absent.includes(problemOf(err).code)) report(err, { action: 'Read the file from disk' })
    return null
  }
}

/**
 * Write a document and answer the ref it resolved to. `encoding: 'base64'` sends
 * a base64 `content` the daemon decodes to raw bytes, so binary writes stay intact.
 */
export async function writeDocument(
  ref: DocumentRef,
  content: string,
  opts?: { encoding: 'base64' },
): Promise<DocumentRef> {
  const owner = ref.authority === 'workspace' ? { workspaceId: ref.workspaceId } : {}
  const written = await apiFetch<DocumentRef>(DOCUMENTS, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ ...owner, path: ref.path, content, ...opts }),
  })
  return toRef(written)
}
