import { useCallback, useMemo, useState } from 'react'
import {
  deleteMessage,
  deleteThread,
  editMessage,
  openThread,
  replyToThread,
  setThreadResolved,
} from '@/features/git/api/review-api'
import { ReviewThreadItem } from '@/features/git/components/review-thread-item'
import { resolveIdentity, useCurrentIdentity } from '@/features/git/hooks/use-current-identity'
import type { GitDiff } from '@/features/git/types/git-types'
import {
  buildUnifiedThreadAnchorMap,
  findNearestUnifiedModelLine,
  findUnifiedModelLine,
  type DiffEditorLineKind,
} from '@/features/git/utils/diff-editor-content'
import { CommentComposer } from '@/features/panes/components/comment-composer'
import type { CommentZoneSpec } from '@/features/editor/components/use-diff-comment-zones'
import { toast } from '@/features/window/stores/toast-store'
import { useWorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'

export interface ReviewCommentLayer {
  commentZones: CommentZoneSpec[]
  onAddCommentAtLine: (modelLine: number) => void
}

/**
 * Builds the inline review-comment layer for ONE file's unified Monaco diff:
 * existing threads → view-zone specs anchored to their model lines, plus a "+"
 * handler that opens an inline composer. Reuses ReviewThreadItem / CommentComposer
 * and the `/threads` REST + WS-stream data layer unchanged — only the rendering
 * host moved from native DOM rows to Monaco view zones.
 *
 * Returns `null` when disabled (non-review diffs) so the editor renders no
 * comment affordances.
 */
export function useReviewCommentLayer(params: {
  enabled: boolean
  diff: GitDiff
  unifiedLineKinds: DiffEditorLineKind[]
}): ReviewCommentLayer | null {
  const { enabled, diff, unifiedLineKinds } = params
  const filePath = diff.file_path
  const wsId = useWorkspaceStoreContext((s) => s.workspaceId)
  const allThreads = useWorkspaceStoreContext((s) => s.branchReview.threads)
  const identity = useCurrentIdentity(wsId)
  const anchors = useMemo(() => buildUnifiedThreadAnchorMap(diff), [diff])
  const [composer, setComposer] = useState<{
    modelLine: number
    side: 'old' | 'new'
    line: number
  } | null>(null)

  const fileThreads = useMemo(
    () => allThreads.filter((t) => t.filePath === filePath),
    [allThreads, filePath],
  )

  // Always stamp the real GitHub login. The identity hook returns null until its
  // fetch resolves, so eagerly await it here — posting before it loaded used to
  // store an empty author (the "U / You" bug).
  const resolveAuthor = useCallback(async () => {
    const id = identity ?? (await resolveIdentity(wsId))
    return id?.login || id?.displayName || undefined
  }, [identity, wsId])

  const handleReply = useCallback(
    async (threadId: string, body: string) => {
      try {
        const author = await resolveAuthor()
        await replyToThread(wsId, threadId, { author, isAgent: false, body })
      } catch (error) {
        toast.error('Failed to post reply', error instanceof Error ? error.message : undefined)
      }
    },
    [resolveAuthor, wsId],
  )
  const handleResolve = useCallback(
    async (threadId: string) => {
      await setThreadResolved(wsId, threadId, true)
    },
    [wsId],
  )
  const handleReopen = useCallback(
    async (threadId: string) => {
      await setThreadResolved(wsId, threadId, false)
    },
    [wsId],
  )
  const handleEditMessage = useCallback(
    async (threadId: string, messageId: string, body: string) => {
      try {
        await editMessage(wsId, threadId, messageId, body)
      } catch (error) {
        toast.error('Failed to edit comment', error instanceof Error ? error.message : undefined)
        // Re-throw so the inline editor stays open and the user keeps their text.
        throw error
      }
    },
    [wsId],
  )
  const handleDeleteMessage = useCallback(
    async (threadId: string, messageId: string) => {
      try {
        await deleteMessage(wsId, threadId, messageId)
      } catch (error) {
        toast.error('Failed to delete comment', error instanceof Error ? error.message : undefined)
      }
    },
    [wsId],
  )
  const handleDeleteThread = useCallback(
    async (threadId: string) => {
      try {
        await deleteThread(wsId, threadId)
      } catch (error) {
        toast.error('Failed to delete thread', error instanceof Error ? error.message : undefined)
      }
    },
    [wsId],
  )
  const handleComposerSubmit = useCallback(
    async (body: string) => {
      if (!composer) return
      try {
        const author = await resolveAuthor()
        await openThread(wsId, {
          filePath,
          line: composer.line,
          startLine: composer.line,
          endLine: composer.line,
          side: composer.side,
          author,
          isAgent: false,
          body,
        })
        setComposer(null)
      } catch (error) {
        toast.error('Failed to post comment', error instanceof Error ? error.message : undefined)
      }
    },
    [composer, filePath, resolveAuthor, wsId],
  )

  const onAddCommentAtLine = useCallback(
    (modelLine: number) => {
      const i = modelLine - 1
      const kind = unifiedLineKinds[i]
      if (!kind || kind === 'spacer') return
      const side: 'old' | 'new' = kind === 'removed' ? 'old' : 'new'
      const entry = anchors[i]
      const line = side === 'old' ? entry?.oldLine : entry?.newLine
      if (line == null) return
      setComposer({ modelLine, side, line })
    },
    [anchors, unifiedLineKinds],
  )

  const commentZones = useMemo<CommentZoneSpec[]>(() => {
    if (!enabled) return []
    const zones: CommentZoneSpec[] = []
    for (const thread of fileThreads) {
      // Anchor to the thread's exact line when it still exists; otherwise the
      // thread is outdated — fall back to the nearest surviving line and render
      // it collapsed (instead of dropping it silently).
      const exactLine = findUnifiedModelLine(anchors, thread.side, thread.lineNumber)
      const isOutdated = exactLine == null
      const modelLine =
        exactLine ?? findNearestUnifiedModelLine(anchors, thread.side, thread.lineNumber)
      if (modelLine == null) continue // no anchored lines on this side at all
      zones.push({
        key: thread.id,
        afterModelLine: modelLine,
        node: (
          <ReviewThreadItem
            thread={thread}
            wsId={wsId}
            currentIdentity={identity}
            isOutdated={isOutdated}
            onReply={handleReply}
            onResolve={handleResolve}
            onReopen={handleReopen}
            onEditMessage={handleEditMessage}
            onDeleteMessage={handleDeleteMessage}
            onDeleteThread={handleDeleteThread}
          />
        ),
      })
    }
    if (composer) {
      zones.push({
        key: `composer:${composer.modelLine}`,
        afterModelLine: composer.modelLine,
        node: (
          <div className="max-w-xl py-2">
            <CommentComposer
              title={`Comment on line ${composer.line}`}
              submitLabel="Comment"
              onSubmit={handleComposerSubmit}
              onCancel={() => setComposer(null)}
            />
          </div>
        ),
      })
    }
    return zones
  }, [
    enabled,
    fileThreads,
    anchors,
    composer,
    wsId,
    identity,
    handleReply,
    handleResolve,
    handleReopen,
    handleEditMessage,
    handleDeleteMessage,
    handleDeleteThread,
    handleComposerSubmit,
  ])

  if (!enabled) return null
  return { commentZones, onAddCommentAtLine }
}
