import { useState } from 'react'
import { CaretDown, CaretRight, TextAlignLeft } from '@phosphor-icons/react'
import { Badge } from '@/components/ui/badge'
import { MarkdownMessageStatic } from '@/features/agent/transcript/plate/markdown-message-static'
import { isGeneralThread } from '@/features/git/components/diff/use-review-annotations'
import { ReviewThreadItem } from '@/features/git/components/review-thread-item'
import { ReviewThreadSnippet } from '@/features/git/components/review-thread-snippet'
import { useReviewThreadActions } from '@/features/git/hooks/use-review-thread-actions'
import { MARKDOWN_PROSE_CLASS } from '@/features/panes/lib/markdown-prose'
import type { ReviewThread } from '@/features/workspace/stores/slices/branch-review-slice'
import { useWorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { ReviewGeneralComposer } from './review-general-composer'

interface ReviewConversationViewProps {
  wsId: string
  /** Switch to Files changed and reveal the thread's line. */
  onJumpToLine: (thread: ReviewThread) => void
}

function ThreadHeader({
  thread,
  open,
  onToggle,
  onJumpToLine,
}: {
  thread: ReviewThread
  open: boolean
  onToggle: () => void
  onJumpToLine: (thread: ReviewThread) => void
}) {
  const general = isGeneralThread(thread)
  const Chevron = open ? CaretDown : CaretRight
  return (
    <header className="flex items-center gap-2 bg-muted/40 px-3 py-2">
      <button
        type="button"
        aria-expanded={open}
        aria-label={open ? 'Collapse thread' : 'Expand thread'}
        className="flex size-5 shrink-0 items-center justify-center rounded text-muted-foreground hover:text-foreground"
        onClick={onToggle}
      >
        <Chevron className="size-3.5" aria-hidden />
      </button>
      {general ? (
        <span className="ui-text-sm text-muted-foreground">General comment</span>
      ) : (
        <span className="ui-text-xs min-w-0 truncate font-mono">
          {thread.filePath}:{thread.lineNumber}
        </span>
      )}
      {thread.isResolved && (
        <Badge variant="secondary" className="shrink-0">
          Resolved
        </Badge>
      )}
      {!general && (
        <button
          type="button"
          className="ui-text-sm ml-auto shrink-0 font-medium text-primary hover:underline"
          onClick={() => onJumpToLine(thread)}
        >
          Jump to line
        </button>
      )}
    </header>
  )
}

/** An open thread shows its code and messages; a resolved one folds to its header until opened. */
function ThreadCard({
  thread,
  wsId,
  actions,
  onJumpToLine,
}: {
  thread: ReviewThread
  wsId: string
  actions: ReturnType<typeof useReviewThreadActions>
  onJumpToLine: (thread: ReviewThread) => void
}) {
  const [toggled, setToggled] = useState<boolean | null>(null)
  const open = toggled ?? !thread.isResolved
  const header = (
    <ThreadHeader
      thread={thread}
      open={open}
      onToggle={() => setToggled(!open)}
      onJumpToLine={onJumpToLine}
    />
  )
  if (!open) {
    return (
      <div className="my-1 overflow-hidden rounded-lg border border-border bg-card opacity-60">
        {header}
      </div>
    )
  }
  return (
    <ReviewThreadItem
      thread={thread}
      wsId={wsId}
      currentIdentity={actions.identity}
      header={
        <>
          {header}
          {!isGeneralThread(thread) && <ReviewThreadSnippet wsId={wsId} thread={thread} />}
        </>
      }
      onReply={actions.reply}
      onResolve={actions.resolve}
      onReopen={actions.reopen}
      onEditMessage={actions.edit}
      onDeleteMessage={actions.removeMessage}
      onDeleteThread={actions.removeThread}
    />
  )
}

/**
 * The branch's conversation: description, every thread, then a general
 * composer. It reads only the threads store, never the file list, so it
 * renders the same on a branch with no changes and on one with a million lines.
 */
export function ReviewConversationView({ wsId, onJumpToLine }: ReviewConversationViewProps) {
  const description = useWorkspaceStoreContext((s) => s.branchReview.description)
  const threads = useWorkspaceStoreContext((s) => s.branchReview.threads)
  const actions = useReviewThreadActions(wsId)
  const { error } = actions

  return (
    <div className="mx-auto flex w-full max-w-[960px] flex-col gap-5 px-4 @lg:px-8 pt-1 pb-16">
      {description.trim() && (
        <section
          aria-label="Description"
          className="overflow-hidden rounded-xl border border-border bg-card"
        >
          <header className="flex items-center gap-2.5 border-b border-border bg-muted/40 px-4 py-2.5">
            <span className="flex size-6 items-center justify-center rounded-full bg-foreground text-background">
              <TextAlignLeft className="size-3.5" aria-hidden />
            </span>
            <span className="ui-text-sm font-semibold">Description</span>
          </header>
          <div className="p-4">
            <MarkdownMessageStatic className={MARKDOWN_PROSE_CLASS}>
              {description}
            </MarkdownMessageStatic>
          </div>
        </section>
      )}

      {threads.length > 0 && (
        <section aria-label="Threads" className="flex flex-col gap-5">
          {threads.map((thread) => (
            <div key={thread.id} className="flex flex-col gap-1.5">
              <ThreadCard
                thread={thread}
                wsId={wsId}
                actions={actions}
                onJumpToLine={onJumpToLine}
              />
              {error?.threadId === thread.id && (
                <p className="ui-text-xs text-destructive">{error.msg}</p>
              )}
            </div>
          ))}
        </section>
      )}

      <ReviewGeneralComposer
        postThread={actions.postThread}
        error={error && error.threadId === undefined ? error.msg : null}
      />
    </div>
  )
}
