import { apiFetch } from '@/lib/api'
import { workspaceBase } from '@/lib/workspace-scope-url'
import type { MultiFileDiff } from '../types/git-diff-types'
import type {
  MergeStrategy,
  ReviewConversation,
  ReviewThread,
} from '@/features/workspace/stores/slices/branch-review-slice'
import type { ThreadDTO, ThreadReplyDTO } from '@/lib/types'

// Branch-review REST client. All routes are workspace-scoped under
// /v0/workspaces/:wsId/review and return the standard {success,data} envelope,
// which apiFetch already unwraps for us (see git-diff-api.ts for the pattern).
//
// Scope: this is the LOCAL branch-vs-parent review surface (description +
// multi-file diff + inline threads + merge strategy). It does NOT create remote
// PRs or execute the merge — those are out of scope for this surface.

function reviewBase(wsId: string): string {
  return `${workspaceBase(wsId)}/review`
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

function mapReply(r: ThreadReplyDTO): ReviewThread['messages'][number] {
  return {
    id: r.id,
    author: r.author || null,
    isAgent: r.isAgent,
    body: r.body,
    createdAt: r.createdAt,
  }
}

/** Map a backend ThreadDTO (from /threads or the WS stream) to the store's ReviewThread.
 *  The root comment body/author/isAgent live at the top level of ThreadDTO; subsequent
 *  messages are in the replies[] array. */
export function mapThread(t: ThreadDTO): ReviewThread {
  const rootMessage: ReviewThread['messages'][number] = {
    id: `${t.id}:root`,
    author: t.author || null,
    isAgent: t.isAgent,
    body: t.body,
    createdAt: t.createdAt,
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

/** Branch-vs-parent multi-file diff. The backend folds this into the review
 *  read model (BranchReview.diff); there is no separate git branch-diff route,
 *  so this derives from the same GET /review payload. */
export async function getBranchDiff(wsId: string): Promise<MultiFileDiff> {
  const raw = await apiFetch<WireBranchReview>(reviewBase(wsId))
  return raw.diff
}

// ── Writes ──────────────────────────────────────────────────────────

/** POST to merge the workspace branch into its parent. Returns void (202 async). */
export async function mergeIntoParent(wsId: string, strategy: MergeStrategy): Promise<void> {
  await apiFetch<unknown>(`${workspaceBase(wsId)}/merge-into-parent`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ strategy }),
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
