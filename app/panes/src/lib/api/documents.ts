import { apiFetch } from '@/lib/api/fetch'
import { problemOf, type Problem, type ProblemCode } from '@/lib/errors/problem'
import { report } from '@/lib/errors/report'
import {
  decodeFileContent,
  type FileContentPayload,
} from '@/features/file-system/utils/file-content-encoding'

export type DocumentRef =
  | { authority: 'workspace'; workspaceId: string; path: string }
  | { authority: 'host'; path: string }

export function workspaceRef(workspaceId: string, path: string): DocumentRef {
  return { authority: 'workspace', workspaceId, path }
}

export type DocumentTarget = DocumentRef | { absolutePath: string }

export interface DocumentRead {
  ref: DocumentRef
  text: string
}

export const HOST_DOCUMENT_TRANSPORT_CODE = 'host_document_requires_local_transport'

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

export async function readDocument(target: DocumentTarget): Promise<DocumentRead> {
  const doc = await apiFetch<DocumentRef & FileContentPayload>(`${DOCUMENTS}?${query(target)}`)
  return { ref: toRef(doc), text: decodeFileContent(doc) }
}

export class WatchRefusal extends Error {
  constructor(readonly problem: Problem) {
    super(problem.message)
    this.name = 'WatchRefusal'
  }
}

export function isWatchLimit(refusal: unknown): boolean {
  return refusal instanceof WatchRefusal && refusal.problem.code === 'watch_limit'
}

export const PREVIEW_ABSENT: readonly ProblemCode[] = [
  'not_found',
  'forbidden',
  HOST_DOCUMENT_TRANSPORT_CODE,
]

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
