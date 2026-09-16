import { useState } from 'react'
import { GitBranch, ArrowUp, ArrowDown } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { cn } from '@/utils/cn'
import { CommitBox } from './commit-box'
import { MergePopover } from './merge-popover'
import { resolveBranchAction } from '../lib/branch-action'
import { pushChanges, pullChanges } from '../api/git-remotes-api'
import { rebaseOntoParent } from '@/lib/api/workspace'
import type { GitFile } from '../types/git-types'

const refresh = () => window.dispatchEvent(new Event('git-status-changed'))

interface BranchSectionProps {
  wsId: string
  parentBranch?: string
  canMergeLocally: boolean
  status: string
  ahead: number
  behind: number
  files: GitFile[]
}

export function BranchSection({
  wsId,
  parentBranch,
  canMergeLocally,
  status,
  ahead,
  behind,
  files,
}: BranchSectionProps) {
  const [remoteBusy, setRemoteBusy] = useState(false)
  const [rebasing, setRebasing] = useState(false)
  const [remoteError, setRemoteError] = useState<string | null>(null)
  const [rebaseError, setRebaseError] = useState<string | null>(null)

  const action = resolveBranchAction({
    hasUncommitted: files.length > 0,
    hasParent: Boolean(parentBranch),
    canMergeLocally,
    status,
    ahead,
    behind,
  })

  // User-initiated "finish the move": rebase the branch onto its parent. Async on
  // the daemon — a clean rebase integrates it; a conflict is kept and surfaces as
  // the conflict state over the WS stream. Crowbar never rebases on its own.
  const handleRebaseOntoParent = async () => {
    setRebasing(true)
    setRebaseError(null)
    try {
      await rebaseOntoParent(wsId)
      refresh()
    } catch (e) {
      setRebaseError(e instanceof Error ? e.message : 'Rebase failed')
    } finally {
      setRebasing(false)
    }
  }

  const runRemote = async (kind: 'push' | 'pull') => {
    setRemoteBusy(true)
    setRemoteError(null)
    try {
      const res = kind === 'push' ? await pushChanges(wsId) : await pullChanges(wsId)
      if (res.success) {
        refresh()
      } else {
        setRemoteError(res.error || `Failed to ${kind}`)
      }
    } catch (e) {
      setRemoteError(e instanceof Error ? e.message : `Failed to ${kind}`)
    } finally {
      setRemoteBusy(false)
    }
  }

  const statusLine = (() => {
    if (remoteBusy && action.remote === 'push') return 'Pushing to remote…'
    if (remoteBusy && action.remote === 'pull') return 'Pulling from remote…'
    if (remoteBusy) return 'Syncing…'
    if (rebasing) return `Rebasing onto ${parentBranch}…`
    if (action.kind === 'commit') {
      return `${files.length} uncommitted change${files.length !== 1 ? 's' : ''}`
    }
    if (action.kind === 'resolve') return `Conflicts with ${parentBranch}`
    if (action.kind === 'pull-request') return `${parentBranch} is protected`
    // Diverged: local and the remote each hold commits the other lacks. Show
    // both — collapsing to just "behind" hides that there's local work to push.
    if (ahead > 0 && behind > 0) return `Diverged · ${ahead} ahead, ${behind} behind`
    if (behind > 0) return `Clean · ${behind} behind`
    if (ahead > 0) return `Clean · ${ahead} to push`
    return 'Up to date'
  })()

  // The commit box (spec 6.3 #1) is always visible — Commit and Pull request
  // sit beside a persistent "Describe the change…" field, not behind a
  // status-dependent trigger.
  //
  // The secondary action row below it only renders when there is something
  // else to do: resolving a conflict, merging into a mergeable parent, or a
  // remote push/pull. The old dedicated "Parent is protected" button is gone —
  // that state is covered by the status line plus the commit box's own
  // (currently backend-less) Pull request button.
  const hasSecondaryAction =
    action.kind === 'resolve' || action.kind === 'merge' || action.remote != null

  return (
    <div className="flex flex-col gap-2 p-3" aria-label="Branch actions">
      <div
        className={cn(
          'ui-text-xs',
          action.kind === 'resolve' ? 'text-destructive' : 'text-muted-foreground',
        )}
      >
        {statusLine}
      </div>

      <CommitBox wsId={wsId} files={files} onCommitted={refresh} />

      {hasSecondaryAction && (
        <div className="flex items-center gap-2">
          {/* The branch conflicts with its parent. The user chooses to rebase onto
              it; on conflict the rebase is kept for the standard resolve flow.
              Crowbar never rebases on its own. */}
          {action.kind === 'resolve' && parentBranch && (
            <Button
              variant="default"
              size="sm"
              className="flex-1"
              disabled={rebasing}
              onClick={() => void handleRebaseOntoParent()}
            >
              <GitBranch className="size-3.5" />
              {rebasing ? 'Rebasing…' : `Rebase onto ${parentBranch}`}
            </Button>
          )}

          {action.kind === 'merge' && parentBranch && (
            <MergePopover
              wsId={wsId}
              parentBranch={parentBranch}
              trigger={
                <Button variant="default" size="sm" className="flex-1">
                  <GitBranch className="size-3.5" />
                  Merge into {parentBranch}
                </Button>
              }
            />
          )}

          {action.remote && (
            <Button
              variant="outline"
              size="sm"
              disabled={remoteBusy}
              onClick={() => void runRemote(action.remote as 'push' | 'pull')}
            >
              {remoteBusy ? (
                <span className="size-3.5 animate-spin rounded-full border border-transparent border-t-current" />
              ) : action.remote === 'push' ? (
                <ArrowUp className="size-3.5" />
              ) : (
                <ArrowDown className="size-3.5" />
              )}
              {remoteBusy
                ? action.remote === 'push'
                  ? 'Pushing…'
                  : 'Pulling…'
                : action.remote === 'push'
                  ? `Push${ahead ? ` ${ahead}` : ''}`
                  : `Pull${behind ? ` ${behind}` : ''}`}
            </Button>
          )}
        </div>
      )}

      {(remoteError || rebaseError) && (
        <p className="ui-text-xs text-destructive mt-1">
          {remoteError ?? rebaseError}
          {' · '}
          <button
            type="button"
            className="underline"
            onClick={() => {
              if (remoteError && (action.remote === 'push' || action.remote === 'pull')) {
                setRemoteError(null)
                void runRemote(action.remote)
              } else if (rebaseError) {
                setRebaseError(null)
                void handleRebaseOntoParent()
              }
            }}
          >
            Retry
          </button>
        </p>
      )}

      {action.kind === 'resolve' && (
        <p className="ui-text-xs text-muted-foreground">
          This branch conflicts with {parentBranch} and isn't integrated yet. Rebase onto{' '}
          {parentBranch} to resolve it — or drag it back to undo.
        </p>
      )}
    </div>
  )
}
