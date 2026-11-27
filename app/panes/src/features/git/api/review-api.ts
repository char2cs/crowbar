import { apiFetch } from '@/lib/api/fetch'
import { reviewBaseForWorkspace as reviewBase, workspaceBase } from '@/lib/workspace-scope-url'
import type { DiffScope } from './review-window-api'
import type { MultiFileDiff } from '../types/git-diff-types'
import type {
  MergeStrategy,
  ReviewConversation,
  ReviewThread,
} from '@/features/workspace/stores/slices/branch-review-slice'
import type { ThreadDTO, ThreadReplyDTO } from '@/lib/types'

interface WireBranchReview {
  description: string
  mergeStrategy: MergeStrategy
  diff: MultiFileDiff
  conversations: WireBranchChat[] | null
}

interface WireBranchChat {
  id: string
  title?: string
  age?: string
  isActive?: boolean
}

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

export function mapThread(t: ThreadDTO): ReviewThread {
  const rootMessage: ReviewThread['messages'][number] = {
    id: t.messageId || `${t.id}:root`,
    author: t.author || null,
    isAgent: t.isAgent,
    body: t.body,
    createdAt: t.createdAt,
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

export async function getReview(wsId: string): Promise<ReviewState> {
  const raw = await apiFetch<WireBranchReview>(reviewBase(wsId))
  return {
    description: raw.description,
    mergeStrategy: raw.mergeStrategy,
    diff: raw.diff,
    conversations: (raw.conversations ?? []).map(mapConversation),
  }
}

export interface ReviewFileSummary {
  path: string
  old_path?: string
  status: 'added' | 'modified' | 'deleted' | 'renamed' | 'untracked'
  additions: number
  deletions: number
  uncommitted: boolean
  staged: boolean
}

export async function getReviewFiles(scope: DiffScope): Promise<ReviewFileSummary[]> {
  const query = scope.commit ? `?sha=${encodeURIComponent(scope.commit)}` : ''
  const raw = await apiFetch<{ files: ReviewFileSummary[] }>(
    `${reviewBase(scope.wsId)}/files${query}`,
  )
  return raw.files ?? []
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

export async function openThread(wsId: string, input: OpenThreadInput): Promise<ReviewThread> {
  const raw = await apiFetch<ThreadDTO>(`${workspaceBase(wsId)}/threads`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
  return mapThread(raw)
}

export async function replyToThread(
  wsId: string,
  threadId: string,
  input: ReplyToThreadInput | string,
): Promise<ReviewThread> {
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

export async function deleteThread(wsId: string, threadId: string): Promise<void> {
  await apiFetch<unknown>(`${workspaceBase(wsId)}/threads/${encodeURIComponent(threadId)}`, {
    method: 'DELETE',
  })
}

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
