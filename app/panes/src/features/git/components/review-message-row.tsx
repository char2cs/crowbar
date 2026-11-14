import { DotsThree, PencilSimple, Copy, Trash } from '@phosphor-icons/react'
import { Badge } from '@/components/ui/badge'
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
} from '@/components/ui/dropdown-menu'
import { CommentComposer } from '@/features/panes/components/comment-composer'
import { MarkdownMessageStatic } from '@/features/agent/transcript/plate/markdown-message-static'
import { MARKDOWN_PROSE_CLASS } from '@/features/panes/lib/markdown-prose'
import { LinkWorkspaceContext } from '@/lib/markdown-link'
import { formatRelativeDate } from '@/utils/date'
import { toast } from '@/features/window/stores/toast-store'
import {
  ChatOrigin,
  MessageAvatar,
  resolveAuthorDisplay,
  useAgentAttribution,
} from '@/features/git/components/review-message-author'
import type { IdentityDTO } from '@/features/git/api/identity-api'
import type { ReviewMessage } from '@/features/workspace/stores/slices/branch-review-slice'

/** Copy a message body to the clipboard as Markdown (the body already is Markdown). */
async function copyAsMarkdown(body: string) {
  try {
    await navigator.clipboard.writeText(body)
    toast.success('Copied to clipboard')
  } catch {
    toast.error('Could not copy to clipboard')
  }
}

export function MessageRow({
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
  const agent = useAgentAttribution(message)
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
          <span className="text-xs text-muted-foreground">
            · {formatRelativeDate(message.createdAt)}
          </span>
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
          <LinkWorkspaceContext value={wsId}>
            <MarkdownMessageStatic variant="chat" className={MARKDOWN_PROSE_CLASS}>
              {message.body}
            </MarkdownMessageStatic>
          </LinkWorkspaceContext>
        )}
      </div>
    </div>
  )
}
