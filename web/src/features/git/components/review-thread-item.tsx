import { useState } from 'react'
import {
  CheckCircle,
  ArrowCounterClockwise,
  DotsThree,
  PencilSimple,
  Copy,
  Trash,
} from '@phosphor-icons/react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Avatar, AvatarImage, AvatarFallback } from '@/components/ui/avatar'
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
} from '@/components/ui/dropdown-menu'
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
import { MarkdownPreview } from '@/features/panes/lib/markdown'
import { toast } from '@/features/window/stores/toast-store'
import type { IdentityDTO } from '@/features/git/api/identity-api'
import type {
  ReviewMessage,
  ReviewThread,
} from '@/features/workspace/stores/slices/branch-review-slice'

export interface ReviewThreadItemProps {
  thread: ReviewThread
  wsId: string
  /** Resolved provider identity, used to show the author's real photo/name. */
  currentIdentity?: IdentityDTO | null
  isOutdated?: boolean
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

interface AuthorDisplay {
  name: string
  login: string | null
  avatarUrl: string | null
  isAgent: boolean
}

// Resolve how to present a message's author. The message carries only the
// provider login string; the current user's full identity (display name +
// avatar) comes from `currentIdentity`. Other logins fall back to the GitHub
// avatar URL convention and the bare login as the name.
function resolveAuthorDisplay(
  message: ReviewMessage,
  currentIdentity: IdentityDTO | null | undefined,
): AuthorDisplay {
  if (message.isAgent) {
    return { name: 'Agent', login: null, avatarUrl: null, isAgent: true }
  }
  const login = message.author ?? null
  if (login && currentIdentity && currentIdentity.login === login) {
    return {
      name: currentIdentity.displayName || login,
      login,
      avatarUrl: currentIdentity.avatarUrl || `https://github.com/${login}.png?size=48`,
      isAgent: false,
    }
  }
  return {
    name: login ?? 'Unknown',
    login,
    avatarUrl: login ? `https://github.com/${login}.png?size=48` : null,
    isAgent: false,
  }
}

function MessageAvatar({ display }: { display: AuthorDisplay }) {
  const initials = (display.name || 'U').slice(0, 2).toUpperCase()
  return (
    <Avatar className="mt-0.5 size-6 shrink-0 text-xs font-semibold">
      {display.avatarUrl && <AvatarImage src={display.avatarUrl} alt={display.name} />}
      <AvatarFallback className="text-xs font-semibold text-muted-foreground">
        {initials}
      </AvatarFallback>
    </Avatar>
  )
}

/** Copy a message body to the clipboard as Markdown (the body already is Markdown). */
async function copyAsMarkdown(body: string) {
  try {
    await navigator.clipboard.writeText(body)
    toast.success('Copied to clipboard')
  } catch {
    toast.error('Could not copy to clipboard')
  }
}

function MessageRow({
  message,
  currentIdentity,
  canEdit,
  canDelete,
  isEditing,
  onStartEdit,
  onRequestDelete,
  onSubmitEdit,
  onCancelEdit,
}: {
  message: ReviewMessage
  currentIdentity: IdentityDTO | null | undefined
  canEdit: boolean
  canDelete: boolean
  isEditing: boolean
  onStartEdit: () => void
  onRequestDelete: () => void
  onSubmitEdit: (body: string) => void
  onCancelEdit: () => void
}) {
  const display = resolveAuthorDisplay(message, currentIdentity)
  // Show the menu only when at least one management action (Edit or Delete) is
  // available for this message; Copy-as-Markdown then rides along inside it.
  const showMenu = !isEditing && (canEdit || canDelete)
  return (
    <div className="group/msg ui-font flex gap-2.5 border-border/60 border-b px-3.5 py-2.5 last:border-b-0">
      <MessageAvatar display={display} />
      <div className="min-w-0 flex-1">
        <div className="mb-1 flex items-baseline gap-1.5">
          <span className="font-semibold text-sm text-foreground">{display.name}</span>
          {display.login && (
            <span className="text-xs text-muted-foreground">@{display.login}</span>
          )}
          {display.isAgent && (
            <Badge variant="outline" className="h-4 border-primary/30 px-1 text-xs text-primary">
              agent
            </Badge>
          )}
          {showMenu && (
            <DropdownMenu>
              <DropdownMenuTrigger
                aria-label="Comment actions"
                className="-my-1 ml-auto inline-flex size-6 shrink-0 items-center justify-center rounded-md text-muted-foreground opacity-0 transition hover:bg-accent hover:text-foreground focus-visible:opacity-100 group-hover/msg:opacity-100 data-[popup-open]:bg-accent data-[popup-open]:opacity-100"
              >
                <DotsThree className="size-4" weight="bold" />
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="min-w-40">
                {canEdit && (
                  <DropdownMenuItem onClick={onStartEdit}>
                    <PencilSimple className="size-4" />
                    Edit
                  </DropdownMenuItem>
                )}
                <DropdownMenuItem onClick={() => void copyAsMarkdown(message.body)}>
                  <Copy className="size-4" />
                  Copy as Markdown
                </DropdownMenuItem>
                {canDelete && (
                  <>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem variant="destructive" onClick={onRequestDelete}>
                      <Trash className="size-4" />
                      Delete
                    </DropdownMenuItem>
                  </>
                )}
              </DropdownMenuContent>
            </DropdownMenu>
          )}
        </div>
        {isEditing ? (
          <div className="py-1">
            <CommentComposer
              initialValue={message.body}
              submitLabel="Save"
              onSubmit={onSubmitEdit}
              onCancel={onCancelEdit}
            />
          </div>
        ) : (
          <MarkdownPreview className="text-sm">{message.body}</MarkdownPreview>
        )}
      </div>
    </div>
  )
}

export function ReviewThreadItem({
  thread,
  currentIdentity,
  isOutdated,
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
        <Badge variant="outline" className="h-4 border-border/40 px-1 text-xs text-muted-foreground/60">
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

  const currentLogin = currentIdentity?.login
  const hasReplies = thread.messages.length > 1
  const deletingIsRoot = pendingDelete?.isRoot ?? false

  return (
    <div
      className={cn(
        'ui-font my-1 max-w-xl rounded-lg border border-border bg-muted/20',
        thread.isResolved && 'opacity-60',
        isOutdated && outdatedExpanded && 'border-border/40',
      )}
    >
      {isOutdated && (
        <div className="flex items-center gap-1.5 border-b border-border/40 px-3 py-1">
          <Badge variant="outline" className="h-4 border-border/40 px-1 text-xs text-muted-foreground/60">
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
            {thread.isResolved ? (
              <Button size="sm" variant="ghost" onClick={handleReopen} disabled={isSubmitting}>
                <ArrowCounterClockwise className="size-3.5" />
                Reopen
              </Button>
            ) : (
              <Button size="sm" variant="default" onClick={handleResolve} disabled={isSubmitting}>
                <CheckCircle className="size-3.5" />
                Resolve
              </Button>
            )}
          </div>
        )}
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
