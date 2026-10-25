import { useState, type ReactNode } from 'react'
import { CheckCircle, ArrowCounterClockwise } from '@phosphor-icons/react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogClose,
} from '@/components/ui/alert-dialog'
import { cn } from '@/utils/cn'
import { CommentComposer } from '@/features/panes/components/comment-composer'
import type { IdentityDTO } from '@/features/git/api/identity-api'
import type { ReviewThread } from '@/features/workspace/stores/slices/branch-review-slice'
import { MessageRow } from './review-message-row'

export interface ReviewThreadItemProps {
  thread: ReviewThread
  /** The workspace this thread belongs to — the scope its agent attribution is
   *  resolved in (providers and chats are per-workspace state) and the one a
   *  message's originating chat is opened in. */
  wsId: string
  /** Resolved provider identity, used to show the author's real photo/name. */
  currentIdentity?: IdentityDTO | null
  isOutdated?: boolean
  /** Card chrome for the conversation view: the header, then one reply-and-resolve footer row. */
  header?: ReactNode
  onReply: (threadId: string, body: string) => Promise<void>
  onResolve: (threadId: string) => Promise<void>
  onReopen: (threadId: string) => Promise<void>
  /** Edit a message body (root or reply) by id. */
  onEditMessage?: (threadId: string, messageId: string, body: string) => Promise<void>
  /** Delete a single reply by id. */
  onDeleteMessage?: (threadId: string, messageId: string) => Promise<void>
  /** Delete the whole thread (used when deleting the root comment). */
  onDeleteThread?: (threadId: string) => Promise<void>
}

export function ReviewThreadItem({
  thread,
  wsId,
  currentIdentity,
  isOutdated,
  header,
  onReply,
  onResolve,
  onReopen,
  onEditMessage,
  onDeleteMessage,
  onDeleteThread,
}: ReviewThreadItemProps) {
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [isReplying, setIsReplying] = useState(false)
  const [outdatedExpanded, setOutdatedExpanded] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  // The message pending deletion (null = no confirmation open). isRoot deletes
  // the whole thread; otherwise a single reply.
  const [pendingDelete, setPendingDelete] = useState<{ messageId: string; isRoot: boolean } | null>(
    null,
  )

  // Show outdated collapsed state
  if (isOutdated && !outdatedExpanded) {
    return (
      <div className="my-1 flex items-center gap-2 px-3 py-1 text-xs text-muted-foreground/40">
        <Badge
          variant="outline"
          className="h-4 border-border/40 px-1 text-xs text-muted-foreground/60"
        >
          Outdated
        </Badge>
        <Button variant="link" size="xs" onClick={() => setOutdatedExpanded(true)}>
          Show
        </Button>
      </div>
    )
  }

  const handleReply = async (body: string) => {
    if (isSubmitting) return
    setIsSubmitting(true)
    try {
      await onReply(thread.id, body)
      setIsReplying(false)
    } finally {
      setIsSubmitting(false)
    }
  }

  const handleResolve = async () => {
    if (isSubmitting || thread.isResolved) return
    setIsSubmitting(true)
    try {
      await onResolve(thread.id)
    } finally {
      setIsSubmitting(false)
    }
  }

  const handleReopen = async () => {
    if (isSubmitting || !thread.isResolved) return
    setIsSubmitting(true)
    try {
      await onReopen(thread.id)
    } finally {
      setIsSubmitting(false)
    }
  }

  const handleSubmitEdit = async (messageId: string, body: string) => {
    try {
      await onEditMessage?.(thread.id, messageId, body)
      setEditingId(null)
    } catch {
      // Keep the editor open (and the user's text) on failure; the handler toasts.
    }
  }

  const handleConfirmDelete = async () => {
    if (!pendingDelete) return
    const { messageId, isRoot } = pendingDelete
    setPendingDelete(null)
    if (isRoot) {
      await onDeleteThread?.(thread.id)
    } else {
      await onDeleteMessage?.(thread.id, messageId)
    }
  }

  const resolveButton = thread.isResolved ? (
    <Button
      size="sm"
      variant="ghost"
      className="shrink-0"
      onClick={handleReopen}
      disabled={isSubmitting}
    >
      <ArrowCounterClockwise className="size-3.5" />
      Reopen
    </Button>
  ) : (
    <Button
      size="sm"
      className="shrink-0"
      variant={header ? 'outline' : 'default'}
      onClick={handleResolve}
      disabled={isSubmitting}
    >
      <CheckCircle className="size-3.5" />
      Resolve
    </Button>
  )

  const currentLogin = currentIdentity?.login
  const hasReplies = thread.messages.length > 1
  const deletingIsRoot = pendingDelete?.isRoot ?? false

  return (
    <div
      className={cn(
        // `ui-font whitespace-normal`: a thread is slotted into the diff's
        // <pre>, and inherits both its font AND its `white-space: pre`. The
        // second one is the quieter bug — every newline between the tags a
        // rendered comment is made of became a visible blank line, so a
        // two-item list arrived with a gap between the bullets wide enough to
        // read as separate paragraphs.
        'ui-font whitespace-normal my-1 rounded-lg border border-border',
        header ? 'w-full overflow-hidden bg-card' : 'max-w-xl bg-muted/20',
        thread.isResolved && 'opacity-60',
        isOutdated && outdatedExpanded && 'border-border/40',
      )}
    >
      {header}
      {isOutdated && (
        <div className="flex items-center gap-1.5 border-b border-border/40 px-3 py-1">
          <Badge
            variant="outline"
            className="h-4 border-border/40 px-1 text-xs text-muted-foreground/60"
          >
            Outdated
          </Badge>
        </div>
      )}

      {/* Messages */}
      <div className="flex flex-col">
        {thread.messages.map((message, index) => {
          const isRoot = index === 0
          const canEdit =
            Boolean(onEditMessage) &&
            !message.isAgent &&
            Boolean(currentLogin) &&
            message.author === currentLogin
          // Deleting the root removes the whole thread; a reply needs the reply handler.
          const canDelete = isRoot ? Boolean(onDeleteThread) : Boolean(onDeleteMessage)
          return (
            <MessageRow
              key={message.id}
              message={message}
              wsId={wsId}
              currentIdentity={currentIdentity}
              canEdit={canEdit}
              canDelete={canDelete}
              isEditing={editingId === message.id}
              onStartEdit={() => setEditingId(message.id)}
              onRequestDelete={() => setPendingDelete({ messageId: message.id, isRoot })}
              onSubmitEdit={(body) => void handleSubmitEdit(message.id, body)}
              onCancelEdit={() => setEditingId(null)}
            />
          )
        })}
      </div>

      {/* Actions */}
      <div className={cn(header && 'border-t border-border')}>
        <div className="px-3 py-2">
          {isReplying ? (
            <CommentComposer
              placeholder="Reply…"
              submitLabel="Reply"
              onSubmit={handleReply}
              onCancel={() => setIsReplying(false)}
            />
          ) : (
            <div className="flex items-center justify-between gap-2">
              {!thread.isResolved && (
                <Input
                  size="sm"
                  readOnly
                  placeholder="Reply…"
                  className="flex-1 cursor-pointer"
                  onClick={() => setIsReplying(true)}
                />
              )}
              {resolveButton}
            </div>
          )}
        </div>
      </div>

      {/* Delete confirmation */}
      <AlertDialog
        open={pendingDelete !== null}
        onOpenChange={(open) => {
          if (!open) setPendingDelete(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {deletingIsRoot ? 'Delete this thread?' : 'Delete this comment?'}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {deletingIsRoot
                ? hasReplies
                  ? 'This permanently deletes the whole thread, including all replies.'
                  : 'This permanently deletes the comment and its thread.'
                : 'This permanently deletes this reply.'}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogClose
              render={
                <Button variant="outline" size="sm">
                  Cancel
                </Button>
              }
            />
            <Button variant="destructive" size="sm" onClick={() => void handleConfirmDelete()}>
              Delete
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
