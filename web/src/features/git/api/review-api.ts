import { apiFetch } from '@/lib/api'
import { workspaceBase } from '@/lib/workspace-scope-url'
import type { MultiFileDiff } from '../types/git-diff-types'
import type {
  MergeStrategy,
  ReviewConversation,
  ReviewThread,
} from '@/features/workspace/stores/slices/branch-review-slice'

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

/** Raw thread message as serialized by the backend (domain.ReviewMessage). */
interface WireReviewMessage {
  id: string
  author?: string
  isAgent: boolean
  body: string
  createdAt: string
}

/** Raw thread as serialized by the backend (domain.ReviewThread). */
interface WireReviewThread {
  id: string
  wsId: string
  filePath: string
  lineNumber: number
  startLine: number
  endLine: number
  side: 'old' | 'new'
  status: 'open' | 'resolved'
  messages: WireReviewMessage[] | null
  createdAt: string
}

/** Raw composite read model (domain.BranchReview). */
interface WireBranchReview {
  description: string
  mergeStrategy: MergeStrategy
  diff: MultiFileDiff
  threads: WireReviewThread[] | null
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

function mapMessage(m: WireReviewMessage): ReviewThread['messages'][number] {
  return {
    id: m.id,
    author: m.author ?? null,
    isAgent: m.isAgent,
    body: m.body,
    createdAt: m.createdAt,
  }
}

export function mapThread(t: WireReviewThread): ReviewThread {
  return {
    id: t.id,
    filePath: t.filePath,
    lineNumber: t.lineNumber,
    startLine: t.startLine ?? t.lineNumber,
    endLine: t.endLine ?? t.lineNumber,
    side: t.side,
    messages: (t.messages ?? []).map(mapMessage),
    isResolved: t.status === 'resolved',
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

/** GET the composite branch-review read model for a workspace. */
export async function getReview(wsId: string): Promise<ReviewState> {
  const raw = await apiFetch<WireBranchReview>(reviewBase(wsId))
  return {
    description: raw.description,
    mergeStrategy: raw.mergeStrategy,
    diff: raw.diff,
    threads: (raw.threads ?? []).map(mapThread),
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
  const raw = await apiFetch<WireReviewThread[]>(`${workspaceBase(wsId)}/threads`)
  return (raw ?? []).map(mapThread)
}

/** POST a new review thread anchored to a file location. Returns the thread. */
export async function openThread(wsId: string, input: OpenThreadInput): Promise<ReviewThread> {
  const raw = await apiFetch<WireReviewThread>(`${workspaceBase(wsId)}/threads`, {
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
  const raw = await apiFetch<WireReviewThread>(
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
  const raw = await apiFetch<WireReviewThread>(
    `${workspaceBase(wsId)}/threads/${encodeURIComponent(threadId)}`,
    {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ isResolved }),
    },
  )
  return mapThread(raw)
}
