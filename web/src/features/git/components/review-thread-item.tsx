import { useState } from 'react'
import { CheckCircle, ArrowCounterClockwise } from '@phosphor-icons/react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Avatar, AvatarImage, AvatarFallback } from '@/components/ui/avatar'
import { cn } from '@/utils/cn'
import { CommentComposer } from '@/features/panes/components/comment-composer'
import { MarkdownPreview } from '@/features/panes/lib/markdown'
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

function MessageRow({
  message,
  currentIdentity,
}: {
  message: ReviewMessage
  currentIdentity: IdentityDTO | null | undefined
}) {
  const display = resolveAuthorDisplay(message, currentIdentity)
  return (
    <div className="ui-font flex gap-2.5 border-border/40 border-b px-3.5 py-2.5 last:border-b-0">
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
        </div>
        <MarkdownPreview className="text-sm">{message.body}</MarkdownPreview>
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
}: ReviewThreadItemProps) {
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [isReplying, setIsReplying] = useState(false)
  const [outdatedExpanded, setOutdatedExpanded] = useState(false)

  // Show outdated collapsed state
  if (isOutdated && !outdatedExpanded) {
    return (
      <div className="my-1 flex items-center gap-2 px-3 py-1 text-xs text-muted-foreground/40">
        <Badge variant="outline" className="h-4 border-border/40 px-1 text-xs text-muted-foreground/60">
          Outdated
        </Badge>
        <button className="underline" onClick={() => setOutdatedExpanded(true)}>
          Show
        </button>
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

  return (
    <div
      className={cn(
        'ui-font my-1 rounded-lg border border-border bg-muted/20',
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
        {thread.messages.map((message) => (
          <MessageRow key={message.id} message={message} currentIdentity={currentIdentity} />
        ))}
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
              <button
                className="flex-1 rounded-md border border-border/60 bg-transparent px-3 py-2 text-left text-sm text-muted-foreground/50 hover:border-border"
                onClick={() => setIsReplying(true)}
              >
                Reply…
              </button>
            )}
            {thread.isResolved ? (
              <Button
                size="xs"
                variant="ghost"
                onClick={handleReopen}
                disabled={isSubmitting}
              >
                <ArrowCounterClockwise className="size-3.5" />
                Reopen
              </Button>
            ) : (
              <Button
                size="xs"
                variant="ghost"
                onClick={handleResolve}
                disabled={isSubmitting}
              >
                <CheckCircle className="size-3.5" />
                Resolve
              </Button>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
