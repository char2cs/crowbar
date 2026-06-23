import { useState } from 'react'
import { GitBranch, ArrowUp, ArrowDown } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { cn } from '@/utils/cn'
import { toast } from '@/features/window/stores/toast-store'
import { CommitPopover } from './commit-popover'
import { MergePopover } from './merge-popover'
import { resolveBranchAction } from '../lib/branch-action'
import { pushChanges, pullChanges } from '../api/git-remotes-api'
import { rebaseOntoParent } from '@/lib/api/workspace'
import type { GitFile } from '../types/git-types'

interface BranchSectionProps {
  wsId: string
  branch: string
  parentBranch?: string
  canMergeLocally: boolean
  status: string
  ahead: number
  behind: number
  files: GitFile[]
}

export function BranchSection({
  wsId,
  branch,
  parentBranch,
  canMergeLocally,
  status,
  ahead,
  behind,
  files,
}: BranchSectionProps) {
  const [remoteBusy, setRemoteBusy] = useState(false)
  const [rebasing, setRebasing] = useState(false)

  const action = resolveBranchAction({
    hasUncommitted: files.length > 0,
    hasParent: Boolean(parentBranch),
    canMergeLocally,
    status,
    ahead,
    behind,
  })

  const refresh = () => window.dispatchEvent(new Event('git-status-changed'))

  // User-initiated "finish the move": rebase the branch onto its parent. Async on
  // the daemon — a clean rebase integrates it; a conflict is kept and surfaces as
  // the conflict state over the WS stream. Crowbar never rebases on its own.
  const handleRebaseOntoParent = async () => {
    setRebasing(true)
    try {
      await rebaseOntoParent(wsId)
      toast.info(`Rebasing onto ${parentBranch}…`)
      refresh()
    } catch (e) {
      toast.error('Rebase failed', e instanceof Error ? e.message : undefined)
    } finally {
      setRebasing(false)
    }
  }

  const runRemote = async (kind: 'push' | 'pull') => {
    setRemoteBusy(true)
    try {
      const res = kind === 'push' ? await pushChanges(wsId) : await pullChanges(wsId)
      if (res.success) {
        // The op runs async on the daemon; the git-status stream reflects the
        // result (updated ahead/behind, or a conflict). Toast the start, then
        // nudge a refresh so the counts resync promptly.
        toast.info(kind === 'push' ? 'Pushing…' : 'Pulling…')
        refresh()
      } else {
        toast.error(res.error || `Failed to ${kind}`)
      }
    } catch (e) {
      toast.error(e instanceof Error ? e.message : `Failed to ${kind}`)
    } finally {
      setRemoteBusy(false)
    }
  }

  const statusLine = (() => {
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

  // The action row only renders when there is something to do: a primary action
  // (anything but the no-parent sync-only state) or a remote (push/pull). A
  // clean, synced, parentless branch shows just the status line — no empty row.
  const hasAction = action.kind !== 'sync-only' || action.remote != null

  return (
    <div className="flex flex-col gap-2 p-3" aria-label="Branch actions">
      <div className="ui-text-sm flex items-center gap-1.5">
        <GitBranch className="size-3.5 text-muted-foreground" />
        <span className="font-mono font-medium">{branch}</span>
        {parentBranch && (
          <>
            <span className="text-muted-foreground">→</span>
            <span className="font-mono text-muted-foreground">{parentBranch}</span>
          </>
        )}
      </div>
      <div
        className={cn(
          'ui-text-xs',
          action.kind === 'resolve' ? 'text-destructive' : 'text-muted-foreground',
        )}
      >
        {statusLine}
      </div>

      {hasAction && (
        <div className="flex items-center gap-2">
          {action.kind === 'commit' && (
            <CommitPopover
              wsId={wsId}
              files={files}
              onCommitted={refresh}
              trigger={
                <Button variant="default" size="sm" className="flex-1">
                  Commit changes
                </Button>
              }
            />
          )}

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

          {/* Protected parents can't be merged locally — informational only (the
              PR is opened on the host). Disabled to avoid implying a click action. */}
          {action.kind === 'pull-request' && (
            <Button variant="outline" size="sm" className="flex-1" disabled>
              {parentBranch} is protected — open a PR
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
              {action.remote === 'push' ? (
                <ArrowUp className="size-3.5" />
              ) : (
                <ArrowDown className="size-3.5" />
              )}
              {action.remote === 'push'
                ? `Push${ahead ? ` ${ahead}` : ''}`
                : `Pull${behind ? ` ${behind}` : ''}`}
            </Button>
          )}
        </div>
      )}

      {action.kind === 'resolve' && (
        <p className="ui-text-xs text-muted-foreground">
          This branch conflicts with {parentBranch} and isn’t integrated yet. Rebase onto{' '}
          {parentBranch} to resolve it — or drag it back to undo.
        </p>
      )}
    </div>
  )
}
