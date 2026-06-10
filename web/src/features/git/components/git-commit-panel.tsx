import { ArrowDown, ArrowUp, WarningCircle as AlertCircle } from '@phosphor-icons/react'
import type React from 'react'
import { useLayoutEffect, useRef, useState } from 'react'
import { Button } from '@/components/ui/button'
import { SidebarComposerBody } from '@/components/ui/sidebar'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import { cn } from '@/utils/cn'
import { commitChanges } from '../api/git-commits-api'
import { pullChanges, pushChanges, type GitRemoteActionResult } from '../api/git-remotes-api'

interface GitCommitPanelProps {
  stagedFilesCount: number
  repoPath?: string
  ahead?: number
  behind?: number
  onCommitSuccess?: () => void
}

const COMMIT_TEXTAREA_MIN_HEIGHT = 64
const COMMIT_TEXTAREA_MAX_HEIGHT = 128

const GitCommitPanel = ({
  stagedFilesCount,
  repoPath,
  ahead = 0,
  behind = 0,
  onCommitSuccess,
}: GitCommitPanelProps) => {
  const [commitMessage, setCommitMessage] = useState('')
  const [isCommitting, setIsCommitting] = useState(false)
  const [remoteAction, setRemoteAction] = useState<'push' | 'pull' | null>(null)
  const [error, setError] = useState<string | null>(null)
  const commitTextareaRef = useRef<HTMLTextAreaElement>(null)

  useLayoutEffect(() => {
    const textarea = commitTextareaRef.current
    if (!textarea) return

    textarea.style.height = 'auto'
    const nextHeight = Math.min(
      COMMIT_TEXTAREA_MAX_HEIGHT,
      Math.max(COMMIT_TEXTAREA_MIN_HEIGHT, textarea.scrollHeight),
    )
    textarea.style.height = `${nextHeight}px`
    textarea.style.overflowY =
      textarea.scrollHeight > COMMIT_TEXTAREA_MAX_HEIGHT ? 'auto' : 'hidden'
  }, [commitMessage])

  const handleCommit = async () => {
    if (!repoPath || !commitMessage.trim() || stagedFilesCount === 0) return

    setIsCommitting(true)
    setError(null)

    try {
      const success = await commitChanges(repoPath, commitMessage.trim())
      if (success) {
        setCommitMessage('')
        onCommitSuccess?.()
      } else {
        setError('Failed to commit changes')
      }
    } catch (error) {
      setError(error instanceof Error ? error.message : 'Unknown error occurred')
    } finally {
      setIsCommitting(false)
    }
  }

  const handleRemoteAction = async (
    action: 'push' | 'pull',
    run: () => Promise<GitRemoteActionResult>,
  ) => {
    if (!repoPath) return

    const label = action === 'push' ? 'Push' : 'Pull'
    let toastId: string | null = null
    setRemoteAction(action)
    setError(null)

    try {
      toastId = toast.show({
        message: `${label}ing changes...`,
        type: 'info',
        duration: 0,
      })

      const result = await run()
      if (result.success) {
        toast.dismiss(toastId)
        toast.success(
          action === 'push' ? 'Changes pushed successfully.' : 'Changes pulled successfully.',
        )
        onCommitSuccess?.()
        return
      }

      const errorMessage = result.error || `Failed to ${action} changes.`
      toast.dismiss(toastId)
      toast.error(errorMessage)
      setError(errorMessage)
    } catch (remoteError) {
      const errorMessage =
        remoteError instanceof Error ? remoteError.message : `Failed to ${action} changes.`
      if (toastId) toast.dismiss(toastId)
      toast.error(errorMessage)
      setError(errorMessage)
    } finally {
      setRemoteAction(null)
    }
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      e.preventDefault()
      void handleCommit()
    }
  }

  const isCommitDisabled = !commitMessage.trim() || stagedFilesCount === 0 || isCommitting
  const hasRemoteChanges = ahead > 0 || behind > 0
  const isRemoteActionLoading = remoteAction !== null
  const composerButtonClassName =
    'h-6 rounded-md border-transparent bg-transparent px-1.5 ui-text-xs leading-none text-muted-foreground shadow-none hover:bg-muted/80 hover:text-foreground focus-visible:ring-1 focus-visible:ring-border-strong/35 [&_svg]:size-3'

  return (
    <>
      <SidebarComposerBody>
        {error && (
          <div
            className={cn(
              'mx-2 mt-2 flex items-center gap-2 rounded border border-error/30',
              'bg-error/20 px-2 py-1 ui-text-xs text-error',
            )}
          >
            <AlertCircle />
            {error}
          </div>
        )}

        <Textarea
          ref={commitTextareaRef}
          id="git-commit-message"
          name="git-commit-message"
          value={commitMessage}
          onChange={(e) => setCommitMessage(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Commit message..."
          variant="ghost"
          className={cn(
            'max-h-32 min-h-16 w-full resize-none overflow-x-hidden bg-transparent',
            'ui-font ui-text-sm px-3 pt-3 pb-2 text-foreground placeholder:text-muted-foreground',
            'focus:outline-none',
          )}
          rows={2}
          disabled={isCommitting}
        />
      </SidebarComposerBody>

      <div className="flex flex-wrap items-center gap-x-2 gap-y-1 px-1 pt-1.5">
        <div className="flex min-w-0 flex-1 flex-wrap items-center gap-1">
          <span className="px-1 ui-text-xs text-muted-foreground">
            {stagedFilesCount > 0
              ? `${stagedFilesCount} file${stagedFilesCount !== 1 ? 's' : ''} staged`
              : 'No files staged'}
          </span>

          {hasRemoteChanges && (
            <div className="flex items-center gap-1">
              {ahead > 0 && (
                <Button
                  type="button"
                  onClick={() => void handleRemoteAction('push', () => pushChanges(repoPath!))}
                  disabled={!repoPath || isRemoteActionLoading}
                  variant="ghost"
                  compact
                  className={cn(composerButtonClassName, 'text-git-added hover:text-git-added')}
                  tooltip={`Push ${ahead} commit${ahead !== 1 ? 's' : ''}`}
                >
                  <ArrowUp />
                  <span>{ahead}</span>
                </Button>
              )}

              {behind > 0 && (
                <Button
                  type="button"
                  onClick={() => void handleRemoteAction('pull', () => pullChanges(repoPath!))}
                  disabled={!repoPath || isRemoteActionLoading}
                  variant="ghost"
                  compact
                  className={cn(composerButtonClassName, 'text-git-deleted hover:text-git-deleted')}
                  tooltip={`Pull ${behind} commit${behind !== 1 ? 's' : ''}`}
                >
                  <ArrowDown />
                  <span>{behind}</span>
                </Button>
              )}
            </div>
          )}
        </div>

        <div className="flex shrink-0 items-center gap-1">
          <Button
            type="button"
            onClick={() => void handleCommit()}
            disabled={isCommitDisabled}
            variant="ghost"
            compact
            className={cn(
              composerButtonClassName,
              isCommitDisabled
                ? 'cursor-not-allowed text-muted-foreground opacity-50'
                : 'text-accent hover:bg-accent/8 hover:text-accent/80',
            )}
          >
            {isCommitting ? 'Committing...' : 'Commit'}
          </Button>
        </div>
      </div>
    </>
  )
}

export default GitCommitPanel
