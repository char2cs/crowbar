import { apiFetch } from '@/lib/api/fetch'
import { operationHeaders, type OperationReply } from '@/core/entities/run-operation'
import {
  reviewBaseForWorkspace as reviewBase,
  workspaceBase,
  worktreeVerbBaseForWorkspace,
} from '@/lib/workspace-scope-url'
import type { DiffScope } from './review-window-api'
import type { MultiFileDiff } from '../types/git-diff-types'
import type {
  MergeStrategy,
  ReviewConversation,
  ReviewThread,
} from '@/features/workspace/stores/slices/branch-review-slice'
import type { ThreadDTO, ThreadReplyDTO } from '@/lib/types'

// The local branch-vs-parent review client. Review routes hang off the worktree's owning chat;
// merge is a worktree lifecycle verb (worktreeVerbBaseForWorkspace). Review-comment /threads are
// a separate, workspace-scoped system. No remote PRs here.

// ── Wire shapes ─────────────────────────────────────────────────────

/** Raw composite read model (domain.BranchReview). Its threads are ignored: they
 *  arrive on the workspace's ws: scope. */
interface WireBranchReview {
  description: string
  mergeStrategy: MergeStrategy
  diff: MultiFileDiff
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

// Missing attribution becomes undefined, not '': the wire omits it on human and older agent
// messages, and '' would look like a real id to every `find` downstream.
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

/** Map a backend ThreadDTO (as the ws: scope streams it) to the UI's ReviewThread.
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
  conversations: ReviewConversation[]
}

// ── Reads ───────────────────────────────────────────────────────────

/** GET the composite branch-review read model for a workspace. */
export async function getReview(wsId: string): Promise<ReviewState> {
  const raw = await apiFetch<WireBranchReview>(reviewBase(wsId))
  return {
    description: raw.description,
    mergeStrategy: raw.mergeStrategy,
    diff: raw.diff,
    conversations: (raw.conversations ?? []).map(mapConversation),
  }
}

/** One path's full change picture (committed and working tree), counts but no line content.
 *  `additions`/`deletions` are -1 for binaries (numstat "-"). */
export interface ReviewFileSummary {
  path: string
  old_path?: string
  status: 'added' | 'modified' | 'deleted' | 'renamed' | 'untracked'
  additions: number
  deletions: number
  uncommitted: boolean
  staged: boolean
}

/** The files-only summary: O(files) with no diff body, so the sidebar's changed-files list can
 *  refetch it on every git-status tick. */
export async function getReviewFiles(scope: DiffScope): Promise<ReviewFileSummary[]> {
  const query = scope.commit ? `?sha=${encodeURIComponent(scope.commit)}` : ''
  const raw = await apiFetch<{ files: ReviewFileSummary[] }>(
    `${reviewBase(scope.wsId)}/files${query}`,
  )
  return raw.files ?? []
}

// ── Writes ──────────────────────────────────────────────────────────

/** Merge the branch into its parent; 202 with the operation under `opId`. deleteSource removes
 *  the merged child after a clean merge (a conflict keeps it). */
export function mergeIntoParent(
  wsId: string,
  strategy: MergeStrategy,
  deleteSource: boolean,
  opId: string,
): Promise<OperationReply> {
  return apiFetch<OperationReply>(`${worktreeVerbBaseForWorkspace(wsId)}/merge-into-parent`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', ...operationHeaders(opId) },
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
  // A plain string is the body alone.
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

/** DELETE a single reply by message id. Returns the updated thread (root + the remaining replies).
 *  The root comment cannot be deleted this way — delete the whole thread instead. */
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
