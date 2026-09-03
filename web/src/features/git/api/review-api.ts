import { apiFetch } from '@/lib/api'
import { reviewBaseForWorkspace, workspaceBase } from '@/lib/workspace-scope-url'
import type { DiffScope } from './review-window-api'
import type { MultiFileDiff } from '../types/git-diff-types'
import type {
  MergeStrategy,
  ReviewConversation,
  ReviewThread,
} from '@/features/workspace/stores/slices/branch-review-slice'
import type { ThreadDTO, ThreadReplyDTO } from '@/lib/types'

// Branch-review REST client. Review routes hang off the chat that owns the
// workspace's worktree (reviewBaseForWorkspace, /v0/chats/:chatId/review) and
// return the standard {success,data} envelope, which apiFetch already unwraps
// for us (see git-diff-api.ts for the pattern). The /threads routes below are
// a separate endpoint group and stay workspace-scoped (workspaceBase) — this
// step does not move them.
//
// Scope: this is the LOCAL branch-vs-parent review surface (description +
// multi-file diff + inline threads + merge strategy). It does NOT create remote
// PRs or execute the merge — those are out of scope for this surface.

function reviewBase(wsId: string): string {
  return reviewBaseForWorkspace(wsId)
}

// ── Wire shapes ─────────────────────────────────────────────────────

// Re-export canonical DTO types so consumers (e.g. use-workspace-threads-stream)
// can import them from a single place without reaching into lib/types directly.
export type { ThreadDTO, ThreadReplyDTO }

/** Raw composite read model (domain.BranchReview).
 *  Note: the `threads` field here comes from the OLD /review composite endpoint
 *  and is intentionally ignored — threads are sourced exclusively from /threads. */
interface WireBranchReview {
  description: string
  mergeStrategy: MergeStrategy
  diff: MultiFileDiff
  threads: unknown[] | null
  conversations: WireBranchChat[] | null
}

/** Raw conversation entry (domain.BranchChat). Shape is loosely typed because
 *  the conversation surface is read-only/stubbed here — see mapConversation. */
interface WireBranchChat {
  id: string
  title?: string
  age?: string
  isActive?: boolean
}

// ── Mappers (wire → store slice types) ──────────────────────────────

// The two attribution fields are carried as '' → undefined rather than through
// verbatim: the wire omits them entirely on a human message and on every agent
// message predating attribution, and an empty string would look like a real id to
// every `find` downstream.
function mapReply(r: ThreadReplyDTO): ReviewThread['messages'][number] {
  return {
    id: r.id,
    author: r.author || null,
    isAgent: r.isAgent,
    body: r.body,
    createdAt: r.createdAt,
    providerId: r.providerId || undefined,
    chatId: r.chatId || undefined,
  }
}

/** Map a backend ThreadDTO (from /threads or the WS stream) to the store's ReviewThread.
 *  The root comment body/author/isAgent live at the top level of ThreadDTO; subsequent
 *  messages are in the replies[] array. */
export function mapThread(t: ThreadDTO): ReviewThread {
  const rootMessage: ReviewThread['messages'][number] = {
    // Prefer the root comment's real id (so it can be edited via /messages/:id);
    // fall back to the synthetic id for any pre-`messageId` payloads.
    id: t.messageId || `${t.id}:root`,
    author: t.author || null,
    isAgent: t.isAgent,
    body: t.body,
    createdAt: t.createdAt,
    // The root's attribution is flattened onto the THREAD, not carried in
    // replies[] — without reading it here an agent-opened thread would render
    // attributed on every reply and anonymous on the finding itself.
    providerId: t.providerId || undefined,
    chatId: t.chatId || undefined,
  }
  return {
    id: t.id,
    filePath: t.filePath,
    lineNumber: t.line,
    startLine: t.startLine || t.line,
    endLine: t.endLine || t.line,
    side: t.side,
    messages: [rootMessage, ...(t.replies ?? []).map(mapReply)],
    isResolved: t.resolved,
  }
}

function mapConversation(c: WireBranchChat): ReviewConversation {
  return {
    id: c.id,
    title: c.title ?? '',
    age: c.age ?? '',
    isActive: c.isActive ?? false,
  }
}

export interface ReviewState {
  description: string
  mergeStrategy: MergeStrategy
  diff: MultiFileDiff
  threads: ReviewThread[]
  conversations: ReviewConversation[]
}

// ── Reads ───────────────────────────────────────────────────────────

/** GET the composite branch-review read model for a workspace.
 *  Threads are intentionally returned as [] — they are sourced exclusively
 *  from /threads + the WS stream (listThreads / useWorkspaceThreadsStream). */
export async function getReview(wsId: string): Promise<ReviewState> {
  const raw = await apiFetch<WireBranchReview>(reviewBase(wsId))
  return {
    description: raw.description,
    mergeStrategy: raw.mergeStrategy,
    diff: raw.diff,
    threads: [],
    conversations: (raw.conversations ?? []).map(mapConversation),
  }
}

/** One entry in the files-only branch-review summary (domain.ReviewFileSummary).
 *  It is the FULL changed-files picture for a path — committed-vs-fork-point AND
 *  working-tree — with +/- counts but NO line content. `additions`/`deletions`
 *  are -1 for binary files (numstat "-" convention). */
export interface ReviewFileSummary {
  path: string
  old_path?: string
  status: 'added' | 'modified' | 'deleted' | 'renamed' | 'untracked'
  additions: number
  deletions: number
  uncommitted: boolean
  staged: boolean
}

/** GET the files-only branch-review summary for a workspace. This is the cheap,
 *  O(file count) source for the always-mounted sidebar changed-files list: it
 *  returns the full branch picture WITHOUT the line-level diff body that
 *  getReview carries, so it can be fetched on every git-status tick without the
 *  refetch loop that made the full diff expensive. */
export async function getReviewFiles(scope: DiffScope): Promise<ReviewFileSummary[]> {
  const query = scope.commit ? `?sha=${encodeURIComponent(scope.commit)}` : ''
  const raw = await apiFetch<{ files: ReviewFileSummary[] }>(
    `${reviewBase(scope.wsId)}/files${query}`,
  )
  return raw.files ?? []
}

// ── Writes ──────────────────────────────────────────────────────────

/**
 * POST to merge the workspace branch into its parent. Returns void (202 async).
 * When deleteSource is true, the daemon removes the now-merged child workspace
 * after a clean merge (a conflict keeps it); the caller redirects to the parent.
 */
export async function mergeIntoParent(
  wsId: string,
  strategy: MergeStrategy,
  deleteSource = false,
): Promise<void> {
  await apiFetch<unknown>(`${workspaceBase(wsId)}/merge-into-parent`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ strategy, deleteSource }),
  })
}

/** PATCH the merge strategy for a workspace. */
export async function setMergeStrategy(
  wsId: string,
  mergeStrategy: MergeStrategy,
): Promise<MergeStrategy> {
  const res = await apiFetch<{ mergeStrategy: MergeStrategy }>(reviewBase(wsId), {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ mergeStrategy }),
  })
  return res.mergeStrategy
}

export interface OpenThreadInput {
  filePath: string
  line: number
  startLine: number
  endLine: number
  side: 'old' | 'new'
  author?: string
  isAgent?: boolean
  body: string
}

export interface ReplyToThreadInput {
  author?: string
  isAgent?: boolean
  body: string
}

/** GET all review threads for a workspace. */
export async function listThreads(wsId: string): Promise<ReviewThread[]> {
  const raw = await apiFetch<ThreadDTO[]>(`${workspaceBase(wsId)}/threads`)
  return (raw ?? []).map(mapThread)
}

/** POST a new review thread anchored to a file location. Returns the thread. */
export async function openThread(wsId: string, input: OpenThreadInput): Promise<ReviewThread> {
  const raw = await apiFetch<ThreadDTO>(`${workspaceBase(wsId)}/threads`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
  return mapThread(raw)
}

/** POST a reply to an existing review thread. Returns the updated thread. */
export async function replyToThread(
  wsId: string,
  threadId: string,
  input: ReplyToThreadInput | string,
): Promise<ReviewThread> {
  // Accept a plain string body for backward compat (old callers pass just the text).
  const payload: ReplyToThreadInput = typeof input === 'string' ? { body: input } : input
  const raw = await apiFetch<ThreadDTO>(
    `${workspaceBase(wsId)}/threads/${encodeURIComponent(threadId)}/replies`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    },
  )
  return mapThread(raw)
}

/** PATCH a thread's resolved state (two-way: pass false to reopen). Returns the updated thread. */
export async function setThreadResolved(
  wsId: string,
  threadId: string,
  isResolved: boolean,
): Promise<ReviewThread> {
  const raw = await apiFetch<ThreadDTO>(
    `${workspaceBase(wsId)}/threads/${encodeURIComponent(threadId)}`,
    {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ isResolved }),
    },
  )
  return mapThread(raw)
}

/** DELETE an entire thread (root comment + replies). The backend forgets the
 *  aggregate and broadcasts a tombstone so the workspace stream drops it. */
export async function deleteThread(wsId: string, threadId: string): Promise<void> {
  await apiFetch<unknown>(`${workspaceBase(wsId)}/threads/${encodeURIComponent(threadId)}`, {
    method: 'DELETE',
  })
}

/** DELETE a single reply by message id. Returns the updated thread (root + the
 *  remaining replies). The root comment cannot be deleted this way — delete the
 *  whole thread instead. */
export async function deleteMessage(
  wsId: string,
  threadId: string,
  messageId: string,
): Promise<ReviewThread> {
  const raw = await apiFetch<ThreadDTO>(
    `${workspaceBase(wsId)}/threads/${encodeURIComponent(threadId)}/messages/${encodeURIComponent(messageId)}`,
    { method: 'DELETE' },
  )
  return mapThread(raw)
}

/** PATCH a message body by id (works for the root comment and replies). Returns
 *  the updated thread. */
export async function editMessage(
  wsId: string,
  threadId: string,
  messageId: string,
  body: string,
): Promise<ReviewThread> {
  const raw = await apiFetch<ThreadDTO>(
    `${workspaceBase(wsId)}/threads/${encodeURIComponent(threadId)}/messages/${encodeURIComponent(messageId)}`,
    {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ body }),
    },
  )
  return mapThread(raw)
}
