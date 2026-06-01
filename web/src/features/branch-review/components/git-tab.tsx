import { useEffect, useRef, useState } from 'react'
import { useGitStore } from '@/features/git/stores/git-store'
import { useRepositoryStore } from '@/features/git/stores/git-repository-store'
import { useSettingsStore } from '@/features/settings/store'
import { useGitFileDiffStats } from '@/features/git/hooks/use-git-file-diff-stats'
import { Separator } from '@/components/ui/separator'
import GitCommitPanel from '@/features/git/components/git-commit-panel'
import GitStatusPanel from '@/features/git/components/status/git-status-panel'
import { GitStashCommandSurface } from '@/features/git/components/git-stash-command-surface'
import GitRemoteManager from '@/features/git/components/git-remote-manager'
import GitTagManager from '@/features/git/components/git-tag-manager'
import { CommitsTab } from './commits-tab'

const WIDE_BREAKPOINT = 480

interface GitTabProps {
  wsId: string
}

export function GitTab({ wsId }: GitTabProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const [isWide, setIsWide] = useState(false)
  const [showStash, setShowStash] = useState(false)
  const [showRemotes, setShowRemotes] = useState(false)
  const [showTags, setShowTags] = useState(false)

  const activeRepoPath = useRepositoryStore.use.activeRepoPath()
  const gitStatus = useGitStore(s => s.gitStatus)
  const showUntracked = useSettingsStore(s => s.settings.showUntrackedFiles)

  const visibleFiles = showUntracked
    ? (gitStatus?.files ?? [])
    : (gitStatus?.files ?? []).filter(f => f.status !== 'untracked')

  const stagedCount = visibleFiles.filter(f => f.staged).length
  const fileDiffStats = useGitFileDiffStats(activeRepoPath, visibleFiles)

  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const observer = new ResizeObserver(([entry]) => {
      setIsWide(entry.contentRect.width >= WIDE_BREAKPOINT)
    })
    observer.observe(el)
    return () => observer.disconnect()
  }, [])

  const stickyCommit = (
    <div className="shrink-0 border-b border-border bg-background px-3 py-2">
      <GitCommitPanel
        stagedFilesCount={stagedCount}
        repoPath={activeRepoPath ?? undefined}
        ahead={gitStatus?.ahead ?? 0}
        behind={gitStatus?.behind ?? 0}
      />
    </div>
  )

  const fileSections = (
    <div className="flex flex-col gap-4 p-3">
      <GitStatusPanel
        files={visibleFiles}
        fileDiffStats={fileDiffStats}
        repoPath={activeRepoPath ?? undefined}
      />
      <GitStashCommandSurface
        isOpen={showStash}
        onClose={() => setShowStash(false)}
        repoPath={activeRepoPath}
        onViewStashDiff={async () => {}}
      />
      <GitRemoteManager
        isOpen={showRemotes}
        onClose={() => setShowRemotes(false)}
        repoPath={activeRepoPath ?? undefined}
      />
      <GitTagManager
        isOpen={showTags}
        onClose={() => setShowTags(false)}
        repoPath={activeRepoPath ?? undefined}
      />
    </div>
  )

  const historySection = (
    <div className="p-3">
      <CommitsTab repoPath={wsId} />
    </div>
  )

  return (
    <div ref={containerRef} className="flex h-full overflow-hidden">
      {isWide ? (
        <>
          <div className="flex flex-1 flex-col overflow-hidden border-r border-border">
            {stickyCommit}
            <div className="flex-1 overflow-y-auto">{fileSections}</div>
          </div>
          <div className="flex-1 overflow-y-auto">{historySection}</div>
        </>
      ) : (
        <div className="flex flex-1 flex-col overflow-hidden">
          {stickyCommit}
          <div className="flex-1 overflow-y-auto">
            {fileSections}
            <Separator className="my-2" />
            {historySection}
          </div>
        </div>
      )}
    </div>
  )
}
