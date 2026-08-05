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
import { useStore } from 'zustand'
import { cn } from '@/utils/cn'
import { CommentComposer } from '@/features/panes/components/comment-composer'
import { MarkdownPreview } from '@/features/panes/lib/markdown'
import { toast } from '@/features/window/stores/toast-store'
import { ProviderIcon } from '@/features/agent/components/provider-icon'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import { openAgentChat } from '@/features/agent/lib/open-agent-chat'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import type { IdentityDTO } from '@/features/git/api/identity-api'
import type {
  ReviewMessage,
  ReviewThread,
} from '@/features/workspace/stores/slices/branch-review-slice'

export interface ReviewThreadItemProps {
  thread: ReviewThread
  /** The workspace this thread belongs to — the scope its agent attribution is
   *  resolved in (providers and chats are per-workspace state) and the one a
   *  message's originating chat is opened in. */
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
  /** Raw SVG for the agent provider that wrote this, when it is one we know. */
  providerIcon: string | null
}

/** What the workspace store could tell us about the agent behind a message.
 *  Every field is '' when the message carries no attribution, when the id names a
 *  provider this workspace has never heard of, or when the chat has since been
 *  deleted — the three cases collapse into "we know nothing", on purpose. */
interface AgentAttribution {
  providerName: string
  providerIcon: string
  /** '' when there is no chat id at all, or the id resolves to no chat. */
  chatTitle: string
  /** The message names a chat that no longer exists — a link would open nothing. */
  chatDeleted: boolean
}

const NO_ATTRIBUTION: AgentAttribution = {
  providerName: '',
  providerIcon: '',
  chatTitle: '',
  chatDeleted: false,
}

// Resolve how to present a message's author. The message carries only the
// provider login string; the current user's full identity (display name +
// avatar) comes from `currentIdentity`. Other logins fall back to the GitHub
// avatar URL convention and the bare login as the name.
//
// An agent message is named by its PROVIDER — resolved by id against the
// workspace's provider list, never by branching on the id itself, so a provider
// Crowbar learns about tomorrow needs no code here. `agent` arrives already
// resolved (and empty when it could not be), which is exactly the historical
// case: every message written before attribution existed carries no provider id,
// and falls through to the generic "Agent" this always showed.
function resolveAuthorDisplay(
  message: ReviewMessage,
  currentIdentity: IdentityDTO | null | undefined,
  agent: AgentAttribution,
): AuthorDisplay {
  if (message.isAgent) {
    return {
      name: agent.providerName || 'Agent',
      login: null,
      avatarUrl: null,
      isAgent: true,
      providerIcon: agent.providerIcon || null,
    }
  }
  const login = message.author ?? null
  if (login && currentIdentity && currentIdentity.login === login) {
    return {
      name: currentIdentity.displayName || login,
      login,
      avatarUrl: currentIdentity.avatarUrl || `https://github.com/${login}.png?size=48`,
      isAgent: false,
      providerIcon: null,
    }
  }
  return {
    name: login ?? 'Unknown',
    login,
    avatarUrl: login ? `https://github.com/${login}.png?size=48` : null,
    isAgent: false,
    providerIcon: null,
  }
}

/**
 * Resolve a message's agent attribution against the workspace store.
 *
 * Three NARROW selectors rather than one returning an object: a selector that
 * built `{name, icon, title}` would hand zustand a fresh object on every store
 * change and re-render the whole thread for a keystroke in an unrelated chat.
 *
 * Nothing here knows any provider's name. An id that resolves to nothing — an
 * absent one, an unknown one, a deleted chat — yields empty strings, and the
 * caller renders exactly what it rendered before attribution existed.
 */
function useAgentAttribution(wsId: string, message: ReviewMessage): AgentAttribution {
  const store = getOrCreateWorkspaceStore(wsId)
  const providerId = message.providerId ?? ''
  const chatId = message.chatId ?? ''

  const providerName = useStore(store, (s) =>
    providerId ? (s.agentChats.providers.find((p) => p.id === providerId)?.displayName ?? '') : '',
  )
  const providerIcon = useStore(store, (s) =>
    providerId ? (s.agentChats.providers.find((p) => p.id === providerId)?.icon ?? '') : '',
  )
  // '' means "no such chat". An untitled chat that still exists is a different
  // state, and wears the same word every other surface gives it.
  const chatTitle = useStore(store, (s) => {
    if (!chatId) return ''
    const chat = s.agentChats.chats.find((c) => c.id === chatId)
    if (!chat) return ''
    return chat.title || UNTITLED_CHAT_LABEL
  })
  // "Deleted" is a CLAIM, and an empty chat list is not evidence for it — the
  // list is seeded asynchronously with the workspace, so a review opened on a
  // cold store would otherwise announce every chat as deleted for as long as the
  // seed took, and then quietly take it back. With no list we know nothing, and
  // the caller shows nothing.
  const chatsKnown = useStore(store, (s) => s.agentChats.chats.length > 0)

  if (!message.isAgent) return NO_ATTRIBUTION
  return {
    providerName,
    providerIcon,
    chatTitle,
    chatDeleted: chatId !== '' && chatTitle === '' && chatsKnown,
  }
}

function MessageAvatar({ display }: { display: AuthorDisplay }) {
  // A known provider wears its own glyph. It is the same mark the chat's pane tab
  // and sidebar row carry, so "who said this" and "where did it come from" read
  // as one thing.
  if (display.providerIcon) {
    return (
      <span className="mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground">
        <ProviderIcon svg={display.providerIcon} className="size-3.5" />
      </span>
    )
  }
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

/**
 * The chat an agent message came out of, as a way back into it.
 *
 * THREE STATES, and they must not look alike. A live chat is a link that reveals
 * or reopens it; a live chat nobody has named yet is that same link wearing the
 * one label every other surface gives it; a chat the user has since DELETED is
 * inert text, because a link that opens nothing is worse than no link. The id
 * outlives the chat — it is stamped on the message forever — so the third state
 * is permanent, not transitional.
 */
function ChatOrigin({
  wsId,
  chatId,
  title,
  deleted,
}: {
  wsId: string
  chatId: string
  title: string
  deleted: boolean
}) {
  if (deleted) {
    return (
      <span
        data-testid="review-message-chat-deleted"
        title="The chat this came from has been deleted"
        className="min-w-0 truncate text-xs text-muted-foreground/60 italic"
      >
        Deleted chat
      </span>
    )
  }
  return (
    <button
      type="button"
      data-testid="review-message-chat-link"
      title={`Open ${title}`}
      onClick={() => openAgentChat(getOrCreateWorkspaceStore(wsId), wsId, chatId)}
      className="min-w-0 max-w-40 truncate text-xs text-muted-foreground underline-offset-2 transition-colors hover:text-foreground hover:underline focus-visible:rounded-sm focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
    >
      {title}
    </button>
  )
}

function MessageRow({
  message,
  wsId,
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
  wsId: string
  currentIdentity: IdentityDTO | null | undefined
  canEdit: boolean
  canDelete: boolean
  isEditing: boolean
  onStartEdit: () => void
  onRequestDelete: () => void
  onSubmitEdit: (body: string) => void
  onCancelEdit: () => void
}) {
  const agent = useAgentAttribution(wsId, message)
  const display = resolveAuthorDisplay(message, currentIdentity, agent)
  // Show the menu only when at least one management action (Edit or Delete) is
  // available for this message; Copy-as-Markdown then rides along inside it.
  const showMenu = !isEditing && (canEdit || canDelete)
  return (
    <div className="group/msg ui-font flex gap-2.5 border-border/60 border-b px-3.5 py-2.5 last:border-b-0">
      <MessageAvatar display={display} />
      <div className="min-w-0 flex-1">
        <div className="mb-1 flex items-baseline gap-1.5">
          <span className="font-semibold text-sm text-foreground">{display.name}</span>
          {display.login && <span className="text-xs text-muted-foreground">@{display.login}</span>}
          {display.isAgent && (
            <Badge variant="outline" className="h-4 border-primary/30 px-1 text-xs text-primary">
              agent
            </Badge>
          )}
          {message.chatId && (agent.chatTitle || agent.chatDeleted) && (
            <ChatOrigin
              wsId={wsId}
              chatId={message.chatId}
              title={agent.chatTitle}
              deleted={agent.chatDeleted}
            />
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
  wsId,
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
        'ui-font whitespace-normal my-1 max-w-xl rounded-lg border border-border bg-muted/20',
        thread.isResolved && 'opacity-60',
        isOutdated && outdatedExpanded && 'border-border/40',
      )}
    >
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
