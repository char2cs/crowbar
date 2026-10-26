import type { ReactNode } from 'react'
import { ChatsCircle, GitBranch } from '@phosphor-icons/react'
import { CHAT_TITLE_TEXT } from '@/components/ui/chat-title-text'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { ProjectIconMark } from '@/components/layout/project-icon-mark'
import { RepoIconMark } from '@/components/layout/repo-icon-mark'
import { formatChangeCount } from '@/components/layout/format-change-count'
import {
  ROW_GLYPH_BOX,
  ROW_SUBLABEL,
  ROW_SUBLABEL_ADD,
  ROW_SUBLABEL_DEL,
} from '@/components/layout/workspace-row-base'
import type { ChatHeaderModel } from '@/features/tabs/hooks/use-chat-branch-header-model'
import { cn } from '@/lib/utils'

interface ChatBranchHeaderViewProps {
  model: ChatHeaderModel
  /** Replaces the title line (the pane's inline rename). */
  titleSlot?: ReactNode
  className?: string
  onDoubleClick?: () => void
}

/**
 * The chat header's drawing, from plain data: the pane's header and the phone's file drawer
 * both render it, so the two show one chat the same way.
 */
export function ChatBranchHeaderView({
  model,
  titleSlot,
  className,
  onDoubleClick,
}: ChatBranchHeaderViewProps) {
  const { title, working, isThread, lead, route } = model
  const glyph = (size: string) => (
    <KindGlyph
      working={working}
      isThread={isThread}
      className={size === 'lg' ? 'size-5' : 'size-[13px]'}
      iconClassName={size === 'lg' ? 'size-[18px]' : 'size-[13px]'}
    />
  )

  return (
    <div
      data-testid="chat-branch-header"
      className={cn('flex items-center gap-2 text-[13px]', className)}
      onDoubleClick={onDoubleClick}
    >
      {lead?.kind === 'repo' ? (
        <span data-testid="chat-branch-header-repo-icon" className="flex shrink-0 items-center">
          <RepoIconMark repo={lead.repo} size="xl" />
        </span>
      ) : lead?.kind === 'project' ? (
        <span data-testid="chat-branch-header-project-icon" className="flex shrink-0 items-center">
          <ProjectIconMark project={lead.project} size="xl" />
        </span>
      ) : (
        title !== null && glyph('lg')
      )}

      <span className="flex min-w-0 flex-1 flex-col justify-center">
        {titleSlot ?? <ChatTitle title={title} className="truncate" />}
        {route?.kind === 'project' && (
          <span className={cn(ROW_SUBLABEL, 'flex items-center gap-1')}>
            {glyph('sm')}
            <span data-testid="chat-branch-header-route" className="min-w-0 truncate">
              {route.name}
            </span>
          </span>
        )}
        {route?.kind === 'branch' && (
          <span className={cn(ROW_SUBLABEL, 'flex items-center gap-1')}>
            {glyph('sm')}
            <WorkspaceRoute
              repoName={route.repoName}
              parentBranch={route.parentBranch}
              branch={route.branch}
            />
            {(route.added > 0 || route.deleted > 0) && (
              <span className="shrink-0 whitespace-pre">
                {' '}
                {route.added > 0 && (
                  <span className={ROW_SUBLABEL_ADD}>+{formatChangeCount(route.added)}</span>
                )}
                {route.added > 0 && route.deleted > 0 && ' '}
                {route.deleted > 0 && (
                  <span className={ROW_SUBLABEL_DEL}>-{formatChangeCount(route.deleted)}</span>
                )}
              </span>
            )}
          </span>
        )}
      </span>
    </div>
  )
}

/** The chat-kind mark (or the working spinner). Where it sits is the caller's
 *  call: left slot when there is no repo to show, else leading the route. */
function KindGlyph({
  working,
  isThread,
  className,
  iconClassName,
}: {
  working: boolean
  isThread: boolean
  className: string
  iconClassName: string
}) {
  return (
    <span data-testid="chat-branch-header-glyph" className={cn(ROW_GLYPH_BOX, className)}>
      {working ? (
        <FlickerSpinner className={iconClassName} />
      ) : isThread ? (
        <ChatsCircle
          data-testid="chat-branch-header-chat-icon"
          className={cn(iconClassName, 'text-muted-foreground')}
          weight="regular"
        />
      ) : (
        <GitBranch
          data-testid="chat-branch-header-branch-icon"
          className={cn(iconClassName, 'text-muted-foreground')}
        />
      )}
    </span>
  )
}

const ROUTE_SEPARATOR = <span className="shrink-0">/</span>

/** `repo / parent / branch` on one line. The parent is the daemon-resolved fork
 *  parent's branch (`Workspace.parentBranch`); the leading segments give up
 *  their width first so the branch, the one that names this chat, stays legible. */
function WorkspaceRoute({
  repoName,
  parentBranch,
  branch,
}: {
  repoName: string | undefined
  parentBranch: string | undefined
  branch: string
}) {
  const parent = parentBranch && parentBranch !== branch ? parentBranch : null
  return (
    <span data-testid="chat-branch-header-route" className="flex min-w-0 items-center">
      {repoName && (
        <>
          <span className="min-w-0 shrink-[1000] truncate">{repoName}</span>
          {ROUTE_SEPARATOR}
        </>
      )}
      {parent && (
        <>
          <span className="min-w-0 shrink-[1000] truncate">{parent}</span>
          {ROUTE_SEPARATOR}
        </>
      )}
      <span className="min-w-0 truncate">{branch}</span>
    </span>
  )
}

/** The name line. A `null` title (a draft, or a store gap) draws nothing, and is
 *  marked as such, never as a chat that happens to be untitled. */
function ChatTitle({ title, className }: { title: string | null; className: string }) {
  return (
    <span
      data-testid="chat-branch-header-title"
      data-title-pending={title === null ? 'true' : undefined}
      className={cn(CHAT_TITLE_TEXT, className)}
    >
      {title}
    </span>
  )
}
