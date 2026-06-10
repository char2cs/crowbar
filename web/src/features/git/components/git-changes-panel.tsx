import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { useGitStore } from '@/features/git/stores/git-store'
import { useSettingsStore } from '@/features/settings/store'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import {
  discardFileChanges,
  stageAllFiles,
  stageFile,
  unstageAllFiles,
  unstageFile,
} from '../api/git-status-api'
import { useGitDiffHandlers } from '../hooks/use-git-diff-handlers'
import GitCommitPanel from './git-commit-panel'
import type { GitFile } from '../types/git-types'

const STATUS_BADGE: Record<string, string> = {
  modified: 'M',
  added: 'A',
  deleted: 'D',
  untracked: 'U',
  renamed: 'R',
  conflicted: 'C',
}

interface FileRowProps {
  file: GitFile
  actionLabel: string
  onAction: () => void
  onOpenDiff: () => void
  onDiscard?: () => void
}

function FileRow({ file, actionLabel, onAction, onOpenDiff, onDiscard }: FileRowProps) {
  return (
    <div className="group mx-1 flex items-center gap-2 rounded-md px-2 py-1 hover:bg-accent">
      <span className="w-4 shrink-0 text-center font-mono text-[11px] text-muted-foreground">
        {STATUS_BADGE[file.status] ?? '?'}
      </span>
      <button
        type="button"
        className="min-w-0 flex-1 cursor-pointer truncate text-left text-[13px]"
        title={`${file.path} — open diff`}
        onClick={onOpenDiff}
      >
        {file.path}
      </button>
      {onDiscard ? (
        <Button
          compact
          variant="ghost"
          className="h-6 px-1.5 text-[11px] opacity-0 group-hover:opacity-100"
          onClick={onDiscard}
        >
          Discard
        </Button>
      ) : null}
      <Button
        compact
        variant="ghost"
        className="h-6 w-6 px-0 text-[13px] opacity-0 group-hover:opacity-100"
        aria-label={`${actionLabel} ${file.path}`}
        onClick={onAction}
      >
        {actionLabel}
      </Button>
    </div>
  )
}

export function GitChangesPanel() {
  const status = useGitStore((s) => s.gitStatus)
  const reload = useGitStore((s) => s.actions.reload)
  const openDiffOnClick = useSettingsStore((s) => s.settings.openDiffOnClick)

  const files = status?.files ?? []
  const staged = files.filter((f) => f.staged)
  const unstaged = files.filter((f) => !f.staged)
  const wsId = getActiveWorkspaceId() ?? ''

  const { handleViewFileDiff } = useGitDiffHandlers({
    activeRepoPath: wsId || null,
    visibleGitFiles: files,
    onFileSelect: (path, isDir) => {
      if (isDir) return
      const rel = wsId && path.startsWith(`${wsId}/`) ? path.slice(wsId.length + 1) : path
      void useFileSystemStore.getState().handleFileOpen?.(rel, false)
    },
  })

  // "Open Diff On Click": open the diff buffer when enabled (default); open
  // the file directly in an editor when the user turned the setting off.
  const openChangedFile = (file: GitFile, staged: boolean) => {
    if (openDiffOnClick) {
      void handleViewFileDiff(file.path, staged)
    } else {
      void useFileSystemStore.getState().handleFileOpen?.(file.path, false)
    }
  }

  const run = async (op: (id: string) => Promise<unknown>) => {
    if (!wsId) return
    await op(wsId)
    await reload(wsId)
  }

  const refresh = () => {
    if (wsId) void reload(wsId)
  }

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      <ScrollArea className="flex-1">
        <div className="py-1">
          {staged.length > 0 ? (
            <section className="mb-1">
              <div className="flex items-center justify-between px-2 py-1">
                <span className="text-[11px] font-medium uppercase text-muted-foreground">
                  Staged ({staged.length})
                </span>
                <Button
                  compact
                  variant="ghost"
                  className="h-6 px-1.5 text-[11px]"
                  onClick={() => void run(unstageAllFiles)}
                >
                  Unstage all
                </Button>
              </div>
              {staged.map((file) => (
                <FileRow
                  key={file.path}
                  file={file}
                  actionLabel="−"
                  onAction={() => void run((id) => unstageFile(id, file.path))}
                  onOpenDiff={() => openChangedFile(file, true)}
                />
              ))}
            </section>
          ) : null}

          {unstaged.length > 0 ? (
            <section>
              <div className="flex items-center justify-between px-2 py-1">
                <span className="text-[11px] font-medium uppercase text-muted-foreground">
                  Changes ({unstaged.length})
                </span>
                <Button
                  compact
                  variant="ghost"
                  className="h-6 px-1.5 text-[11px]"
                  onClick={() => void run(stageAllFiles)}
                >
                  Stage all
                </Button>
              </div>
              {unstaged.map((file) => (
                <FileRow
                  key={file.path}
                  file={file}
                  actionLabel="+"
                  onAction={() => void run((id) => stageFile(id, file.path))}
                  onOpenDiff={() => openChangedFile(file, false)}
                  onDiscard={() => void run((id) => discardFileChanges(id, file.path))}
                />
              ))}
            </section>
          ) : null}

          {files.length === 0 ? (
            <div className="px-2 py-6 text-center text-[13px] text-muted-foreground">
              No changes
            </div>
          ) : null}
        </div>
      </ScrollArea>

      <div className="shrink-0 border-t border-border">
        <GitCommitPanel
          stagedFilesCount={staged.length}
          repoPath={wsId}
          ahead={status?.ahead ?? 0}
          behind={status?.behind ?? 0}
          onCommitSuccess={refresh}
        />
      </div>
    </div>
  )
}
