import { useCallback, useState } from 'react'
import {
  deleteMessage,
  deleteThread,
  editMessage,
  openThread,
  replyToThread,
  setThreadResolved,
} from '@/features/git/api/review-api'
import type { OpenThreadInput } from '@/features/git/api/review-api'
import { resolveIdentity, useCurrentIdentity } from '@/features/git/hooks/use-current-identity'

export interface ReviewThreadError {
  threadId?: string
  msg: string
}

/** Where a new thread hangs; the author and the human flag are stamped here. */
export type NewThreadInput = Omit<OpenThreadInput, 'author' | 'isAgent'>

/**
 * Every review-thread write, shared by the diff annotations and the
 * Conversation view. There is deliberately no optimistic state: the threads
 * store is updated by the workspace WS stream, so a successful call needs no
 * local bookkeeping and a failed one only records `error`.
 */
export function useReviewThreadActions(wsId: string) {
  const identity = useCurrentIdentity(wsId)
  const [error, setError] = useState<ReviewThreadError | null>(null)

  // Always stamp the real provider login. The identity hook returns null until
  // its fetch resolves, so eagerly await it here — posting before it loaded used
  // to store an empty author (the "U / You" bug).
  const resolveAuthor = useCallback(async () => {
    const id = identity ?? (await resolveIdentity(wsId))
    return id?.login || id?.displayName || undefined
  }, [identity, wsId])

  const clearError = useCallback(() => setError(null), [])

  const postThread = useCallback(
    async (input: NewThreadInput): Promise<boolean> => {
      setError(null)
      try {
        const author = await resolveAuthor()
        await openThread(wsId, { ...input, author, isAgent: false })
        return true
      } catch {
        setError({ msg: 'Failed to post comment' })
        return false
      }
    },
    [resolveAuthor, wsId],
  )

  const reply = useCallback(
    async (threadId: string, body: string) => {
      setError(null)
      try {
        const author = await resolveAuthor()
        await replyToThread(wsId, threadId, { author, isAgent: false, body })
      } catch {
        setError({ threadId, msg: 'Failed to post reply' })
      }
    },
    [resolveAuthor, wsId],
  )

  const resolve = useCallback(
    async (threadId: string) => {
      await setThreadResolved(wsId, threadId, true)
    },
    [wsId],
  )

  const reopen = useCallback(
    async (threadId: string) => {
      await setThreadResolved(wsId, threadId, false)
    },
    [wsId],
  )

  const edit = useCallback(
    async (threadId: string, messageId: string, body: string) => {
      setError(null)
      try {
        await editMessage(wsId, threadId, messageId, body)
      } catch (err) {
        setError({ threadId, msg: 'Failed to edit comment' })
        // Re-throw so the inline editor stays open and the user keeps their text.
        throw err
      }
    },
    [wsId],
  )

  const removeMessage = useCallback(
    async (threadId: string, messageId: string) => {
      setError(null)
      try {
        await deleteMessage(wsId, threadId, messageId)
      } catch {
        setError({ threadId, msg: 'Failed to delete comment' })
      }
    },
    [wsId],
  )

  const removeThread = useCallback(
    async (threadId: string) => {
      setError(null)
      try {
        await deleteThread(wsId, threadId)
      } catch {
        setError({ threadId, msg: 'Failed to delete thread' })
      }
    },
    [wsId],
  )

  return {
    identity,
    error,
    clearError,
    postThread,
    reply,
    resolve,
    reopen,
    edit,
    removeMessage,
    removeThread,
  }
}
