import { useEffect, useState } from 'react'
import { getGitLog } from '@/features/git/api/git-commits-api'
import type { GitCommit } from '@/features/git/types/git-types'
import { formatRelativeDate } from '@/utils/date'
import { Frame, FramePanel, FrameHeader, FrameTitle } from '@/components/ui/frame'

interface CommitsTabProps {
  repoPath: string
}

export function CommitsTab({ repoPath }: CommitsTabProps) {
  const [commits, setCommits] = useState<GitCommit[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    getGitLog(repoPath, 50).then(result => {
      if (!cancelled) { setCommits(result); setLoading(false) }
    })
    return () => { cancelled = true }
  }, [repoPath])

  return (
    <div className="p-4">
      <Frame>
        <FramePanel>
          <FrameHeader className="px-0 pt-0 pb-3">
            <FrameTitle>Commit history</FrameTitle>
          </FrameHeader>
          {loading ? (
            <p className="text-xs text-muted-foreground/40">Loading commits…</p>
          ) : commits.length === 0 ? (
            <p className="text-xs text-muted-foreground/40">No commits on this branch yet.</p>
          ) : (
            <div className="flex flex-col gap-0.5">
              {commits.map(commit => (
                <div key={commit.hash} className="flex items-center gap-2.5 rounded-lg px-3 py-2 hover:bg-accent transition-colors">
                  <span className="w-12 shrink-0 font-mono text-[10px] text-muted-foreground/60">{commit.hash.slice(0, 7)}</span>
                  <span className="flex-1 truncate text-xs text-foreground">{commit.message}</span>
                  <span className="shrink-0 text-[10px] text-muted-foreground/50">{formatRelativeDate(commit.date)}</span>
                </div>
              ))}
            </div>
          )}
        </FramePanel>
      </Frame>
    </div>
  )
}
