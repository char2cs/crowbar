import { useCallback, useEffect } from 'react'
import { formatRelativeDate } from '@/utils/date'
import { FramePanel, FrameTitle } from '@/components/ui/frame'
import { DataState } from '@/components/ui/data-state'
import { useGitStore } from '@/features/git/stores/git-store'

interface CommitsTabProps {
  repoPath: string
}

export function CommitsTab({ repoPath }: CommitsTabProps) {
  const gitData = useGitStore(s => s.gitData)
  const retry = useCallback(() => { void useGitStore.getState().fetchGitData(repoPath) }, [repoPath])

  useEffect(() => { void useGitStore.getState().fetchGitData(repoPath) }, [repoPath])

  return (
    <div className="flex flex-col gap-4">
      <FrameTitle className="text-base">Commit history</FrameTitle>
      <DataState
        loadable={gitData}
        onRetry={retry}
        loadingLabel="Loading commits"
        emptyMessage="No commits yet"
        isEmpty={(d) => d.commits.length === 0}
      >
        {({ commits }) => (
          <div className="flex flex-col gap-1.5">
            {commits.map(commit => (
              <FramePanel key={commit.hash} className="py-2 px-3">
                <div className="flex items-center gap-3">
                  <span className="w-12 shrink-0 font-mono text-[10px] text-muted-foreground/60">{commit.hash.slice(0, 7)}</span>
                  <span className="flex-1 truncate text-sm text-foreground">{commit.message}</span>
                  <span className="shrink-0 text-xs text-muted-foreground/50">{formatRelativeDate(commit.date)}</span>
                </div>
              </FramePanel>
            ))}
          </div>
        )}
      </DataState>
    </div>
  )
}
