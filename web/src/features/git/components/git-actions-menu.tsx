import {
  Archive,
  Download,
  FolderOpen,
  GitPullRequest,
  ArrowClockwise as RefreshCw,
  ArrowCounterClockwise as RotateCcw,
  HardDrives as Server,
  GearSix as Settings,
  Tag,
  Upload,
} from '@phosphor-icons/react'
import { useState } from 'react'
import { useSettingsStore } from '@/features/settings/store'
import { ContextMenu, type ContextMenuItem } from '@/components/ui/context-menu'
import { LoadingSpinner } from '@/components/ui/loading-spinner'
import { primitiveConfirm } from '@/components/ui/primitive-dialog-service'
import {
  fetchChanges,
  pullChanges,
  pushChanges,
  type GitRemoteActionResult,
} from '../api/git-remotes-api'
import { discardAllChanges } from '../api/git-status-api'
import { useGitStore } from '../stores/git-store'
import { type GitActionsMenuAnchorRect } from '../utils/git-actions-menu-position'

interface GitActionsMenuProps {
  isOpen: boolean
  anchorRect: GitActionsMenuAnchorRect | null
  onClose: () => void
  hasGitRepo: boolean
  repoPath?: string
  onRefresh?: () => void
  onOpenRemoteManager?: () => void
  onOpenTagManager?: () => void
  onViewStashes?: () => void
  onSelectRepository?: () => Promise<void> | void
  isSelectingRepository?: boolean
  onInitializeRepository?: () => Promise<void> | void
  isInitializingRepository?: boolean
}

const GitActionsMenu = ({
  isOpen,
  anchorRect,
  onClose,
  hasGitRepo,
  repoPath,
  onRefresh,
  onOpenRemoteManager,
  onOpenTagManager,
  onViewStashes,
  onSelectRepository,
  isSelectingRepository,
  onInitializeRepository,
  isInitializingRepository,
}: GitActionsMenuProps) => {
  const [runningAction, setRunningAction] = useState<string | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)
  const isRefreshing = useGitStore((s) => s.isRefreshing)
  const confirmBeforeDiscard = useSettingsStore((state) => state.settings.confirmBeforeDiscard)

  const handleAction = async (
    action: () => Promise<boolean | GitRemoteActionResult>,
    actionName: string,
    messages?: {
      success?: string
      error?: string
    },
  ) => {
    if (!repoPath) return

    setRunningAction(actionName)
    setActionError(null)
    try {
      const result = await action()
      const remoteResult =
        typeof result === 'boolean' ? { success: result, error: undefined } : result

      if (remoteResult.success) {
        onRefresh?.()
        onClose()
      } else {
        const errorMessage = remoteResult.error || messages?.error || `${actionName} failed.`
        console.error(`${actionName} failed`, remoteResult.error)
        setActionError(errorMessage)
      }
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : messages?.error || `${actionName} failed.`
      console.error(`${actionName} error:`, error)
      setActionError(errorMessage)
    } finally {
      setRunningAction(null)
    }
  }

  const handlePush = () => {
    void handleAction(() => pushChanges(repoPath!), 'Push', {
      error: 'Failed to push changes.',
    })
  }

  const handlePull = () => {
    void handleAction(() => pullChanges(repoPath!), 'Pull', {
      error: 'Failed to pull changes.',
    })
  }

  const handleFetch = () => {
    void handleAction(() => fetchChanges(repoPath!), 'Fetch', {
      error: 'Failed to fetch changes.',
    })
  }

  const handleDiscardAllChanges = async () => {
    if (!repoPath) return
    if (
      confirmBeforeDiscard &&
      !(await primitiveConfirm('Discard all unstaged changes? This cannot be undone.', {
        title: 'Discard Changes',
        confirmLabel: 'Discard',
      }))
    ) {
      return
    }
    void handleAction(() => discardAllChanges(repoPath!), 'Discard all changes')
  }

  const handleInitRepository = () => {
    if (onInitializeRepository) {
      void onInitializeRepository()
      onClose()
      return
    }

    void handleAction(() => Promise.resolve(false), 'Initialize repository')
  }

  const handleRefresh = async () => {
    await onRefresh?.()
  }

  const handleRemoteManager = () => {
    onOpenRemoteManager?.()
    onClose()
  }

  const handleTagManager = () => {
    onOpenTagManager?.()
    onClose()
  }

  const handleViewStashes = () => {
    onViewStashes?.()
    onClose()
  }

  const handleSelectRepository = async () => {
    await onSelectRepository?.()
    onClose()
  }

  if (!isOpen || !anchorRect) {
    return null
  }

  const spinner = (
    <span className="size-3.5 animate-spin rounded-full border border-transparent border-t-current" />
  )

  const errorFooter = actionError ? (
    <div className="px-2 py-1.5 text-xs text-destructive border-t border-border mt-1">
      {actionError}
    </div>
  ) : null

  const items: ContextMenuItem[] = hasGitRepo
    ? [
        {
          id: 'select-repository',
          label: isSelectingRepository ? 'Selecting...' : 'Select Repository',
          icon: <FolderOpen />,
          disabled: isSelectingRepository,
          onClick: () => void handleSelectRepository(),
        },
        { id: 'sep-1', label: '', separator: true, onClick: () => {} },
        {
          id: 'push',
          label: 'Push Changes',
          icon: runningAction === 'Push' ? spinner : <Upload />,
          disabled: !!runningAction,
          closeOnClick: false,
          onClick: handlePush,
        },
        { id: 'sep-2', label: '', separator: true, onClick: () => {} },
        {
          id: 'pull',
          label: 'Pull Changes',
          icon: runningAction === 'Pull' ? spinner : <Download />,
          disabled: !!runningAction,
          closeOnClick: false,
          onClick: handlePull,
        },
        {
          id: 'fetch',
          label: 'Fetch',
          icon: runningAction === 'Fetch' ? spinner : <GitPullRequest />,
          disabled: !!runningAction,
          closeOnClick: false,
          onClick: handleFetch,
        },
        { id: 'sep-3', label: '', separator: true, onClick: () => {} },
        {
          id: 'manage-remotes',
          label: 'Manage Remotes',
          icon: <Server />,
          onClick: handleRemoteManager,
        },
        {
          id: 'manage-tags',
          label: 'Manage Tags',
          icon: <Tag />,
          onClick: handleTagManager,
        },
        {
          id: 'view-stashes',
          label: 'View Stashes',
          icon: <Archive />,
          onClick: handleViewStashes,
        },
        { id: 'sep-4', label: '', separator: true, onClick: () => {} },
        {
          id: 'refresh',
          label: 'Refresh Status',
          icon: isRefreshing ? <LoadingSpinner label="Refreshing status" compact /> : <RefreshCw />,
          disabled: isRefreshing,
          onClick: () => void handleRefresh(),
        },
        { id: 'sep-5', label: '', separator: true, onClick: () => {} },
        {
          id: 'discard-all',
          label: 'Discard All Changes',
          icon: runningAction === 'Discard all changes' ? spinner : <RotateCcw />,
          disabled: !!runningAction,
          closeOnClick: false,
          className: 'text-red-400',
          onClick: () => void handleDiscardAllChanges(),
        },
      ]
    : [
        {
          id: 'init-repository',
          label: isInitializingRepository ? 'Initializing...' : 'Initialize Repository',
          icon: <Settings />,
          disabled: !!runningAction || isInitializingRepository,
          onClick: handleInitRepository,
        },
        { id: 'sep-1', label: '', separator: true, onClick: () => {} },
        {
          id: 'refresh',
          label: 'Refresh Status',
          icon: isRefreshing ? <LoadingSpinner label="Refreshing status" compact /> : <RefreshCw />,
          disabled: isRefreshing,
          onClick: () => void handleRefresh(),
        },
      ]

  return (
    <ContextMenu
      isOpen={isOpen}
      position={{
        x: anchorRect.right,
        y: anchorRect.bottom + 6,
      }}
      items={items}
      onClose={onClose}
      footer={errorFooter}
    />
  )
}

export default GitActionsMenu
