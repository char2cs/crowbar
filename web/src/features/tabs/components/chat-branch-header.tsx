import { ChatsCircle, GitBranch } from '@phosphor-icons/react'
import { useState } from 'react'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { performRenameChat } from '@/components/sidebar/lib/row-actions'
import { InlineRenameInput } from '@/components/sidebar/inline-rename-input'
import { ProjectIconMark } from '@/components/layout/project-icon-mark'
import { RepoIconMark } from '@/components/layout/repo-icon-mark'
import { formatChangeCount } from '@/components/layout/format-change-count'
import {
  ROW_GLYPH_BOX,
  ROW_SUBLABEL,
  ROW_SUBLABEL_ADD,
  ROW_SUBLABEL_DEL,
} from '@/components/layout/workspace-row-base'
import { CHAT_TITLE_PENDING_LABEL, useChatTitle } from '@/features/agent/hooks/use-chat-title'
import { useChatIsThread } from '@/features/panes/hooks/use-chat-is-thread'
import { useWorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { dataOf } from '@/lib/loadable'
import { useHomeTreeStore, type HomeTree } from '@/lib/store/home-tree'
import { useProjectDataStore } from '@/lib/store/projects'
import { useSidebarStore } from '@/lib/store/sidebar'
import { cn } from '@/lib/utils'

interface ChatBranchHeaderProps {
  chatId: string
  /** The chat's own workspace (see pane-container.tsx's `wsId` resolution) —
   *  `null` when it hasn't resolved yet, in which case only the title shows. */
  wsId: string | null
  /** No fixed height/padding of its own — this renders in two different
   *  contexts (its own row atop the chat view, or nested inside
   *  ChatOnlyPaneHeader's row alongside other buttons) that size it
   *  differently. The caller owns sizing entirely. */
  className?: string
}

/**
 * The chat interface's own identity header — replaces `ChatHead` at this
 * position now that the chat and the IDE sector are two separate boxes with
 * their own headers. Deliberately NOT a `Tab`: this isn't one of several
 * things to switch between here, so it carries none of the sidebar row's
 * hover/selection chrome, just its rule-6 two-line shape (title, then branch
 * name + change counts) — see sidebar-row.tsx's `BranchSecondLine` for the
 * row this mirrors.
 *
 * Rename is double-click, matching the sidebar's own convention
 * (`sidebar-row.tsx`), routed through the same `performRenameChat` action —
 * a chat's name has one write path regardless of which surface edits it.
 */
export function ChatBranchHeader({ chatId, wsId, className }: ChatBranchHeaderProps) {
  // `null` = this store has no record for the chat — a gap, not a name, so the
  // header shows the pending mark and offers no rename (there is no current
  // title to edit from).
  const title = useChatTitle(chatId)
  const working = useWorkspaceStoreContext((s) => s.agentChats.working[chatId] ?? false)
  const repo = useSidebarStore((s) =>
    wsId ? (s.repos.find((r) => r.workspaces.some((w) => w.id === wsId)) ?? null) : null,
  )
  const workspace = repo?.workspaces.find((w) => w.id === wsId) ?? null
  // A project-home chat rides no repo (home-tree.ts), so the chat's own
  // project id comes from the home tree that lists it, never from `wsId`.
  const homeProjectId = useHomeTreeStore((s) => homeProjectIdOf(s.trees, chatId))
  const project = useProjectDataStore((s) =>
    homeProjectId ? (dataOf(s.data)?.find((p) => p.id === homeProjectId) ?? null) : null,
  )
  // A thread owns no branch — it runs on the one its parent owns — so it gets
  // the bubble, never the branch mark, exactly as `RowGlyph` (sidebar-row.tsx)
  // already draws it in the tree. `wsId` above cannot answer this: a thread
  // inherits its parent's workspace id.
  const isThread = useChatIsThread(chatId)
  const [renaming, setRenaming] = useState(false)

  const routed = workspace?.branch ? workspace : null
  const added = workspace?.added ?? 0
  const deleted = workspace?.deleted ?? 0

  return (
    <div
      data-testid="chat-branch-header"
      className={cn('flex items-center gap-2 text-[13px]', className)}
      onDoubleClick={() => {
        if (title !== null) setRenaming(true)
      }}
    >
      {routed && repo ? (
        <span data-testid="chat-branch-header-repo-icon" className="flex shrink-0 items-center">
          <RepoIconMark repo={repo} size="xl" />
        </span>
      ) : project && !routed ? (
        <span data-testid="chat-branch-header-project-icon" className="flex shrink-0 items-center">
          <ProjectIconMark project={project} size="xl" />
        </span>
      ) : (
        <KindGlyph
          working={working}
          isThread={isThread}
          className="size-5"
          iconClassName="size-[18px]"
        />
      )}

      <span className="flex min-w-0 flex-1 flex-col justify-center">
        {renaming && title !== null ? (
          <InlineRenameInput
            defaultValue={title}
            className={CHAT_TITLE_TEXT}
            onConfirm={(name) => {
              setRenaming(false)
              if (name !== title) void performRenameChat(chatId, name)
            }}
            onCancel={() => setRenaming(false)}
          />
        ) : (
          <ChatTitle title={title} className="truncate" />
        )}
        {project && !routed && (
          <span className={cn(ROW_SUBLABEL, 'flex items-center gap-1')}>
            <KindGlyph
              working={working}
              isThread={isThread}
              className="size-[13px]"
              iconClassName="size-[13px]"
            />
            <span data-testid="chat-branch-header-route" className="min-w-0 truncate">
              {project.name}
            </span>
          </span>
        )}
        {routed && (
          <span className={cn(ROW_SUBLABEL, 'flex items-center gap-1')}>
            {repo && (
              <KindGlyph
                working={working}
                isThread={isThread}
                className="size-[13px]"
                iconClassName="size-[13px]"
              />
            )}
            <WorkspaceRoute
              repoName={repo?.name}
              parentBranch={routed.parentBranch}
              branch={routed.branch}
            />
            {(added > 0 || deleted > 0) && (
              <span className="shrink-0 whitespace-pre">
                {' -- '}
                {added > 0 && <span className={ROW_SUBLABEL_ADD}>+{formatChangeCount(added)}</span>}
                {added > 0 && deleted > 0 && ' '}
                {deleted > 0 && (
                  <span className={ROW_SUBLABEL_DEL}>-{formatChangeCount(deleted)}</span>
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

function homeProjectIdOf(trees: Record<string, HomeTree>, chatId: string): string | null {
  for (const projectId of Object.keys(trees)) {
    if (trees[projectId].chats.some((c) => c.id === chatId)) return projectId
  }
  return null
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

// Shared with the rename input so entering edit mode never changes the text.
const CHAT_TITLE_TEXT = 'font-heading text-xl leading-5 font-normal tracking-normal'

/** The name line. A `null` title is the store's gap, drawn as the pending mark
 *  and marked as such, never as a chat that happens to be untitled. */
function ChatTitle({ title, className }: { title: string | null; className: string }) {
  return (
    <span
      data-testid="chat-branch-header-title"
      data-title-pending={title === null ? 'true' : undefined}
      className={cn(CHAT_TITLE_TEXT, className)}
    >
      {title ?? CHAT_TITLE_PENDING_LABEL}
    </span>
  )
}
