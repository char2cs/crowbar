import { useEffect, useState } from 'react'
import { ScrollArea } from '@/components/ui/scroll-area'
import { apiFetch } from '@/lib/api'

interface Commit {
  hash: string
  shortHash: string
  message: string
  author: string
  date: string
}

export function GitHistoryList() {
  const [commits, setCommits] = useState<Commit[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    apiFetch<Commit[]>('/api/v0/git/log?limit=50')
      .then(setCommits)
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  if (loading) {
    return <div className="flex flex-1 items-center justify-center text-[13px] text-muted-foreground">Loading…</div>
  }

  return (
    <ScrollArea className="flex-1">
      <div className="py-1">
        {commits.map(commit => (
          <div key={commit.hash} className="flex items-start gap-2 mx-1.5 my-0.5 px-2 py-1.5 hover:bg-accent rounded-md cursor-pointer">
            <span className="mt-0.5 shrink-0 font-mono text-[11px] text-muted-foreground">{commit.shortHash}</span>
            <div className="min-w-0 flex-1">
              <p className="truncate text-[13px]">{commit.message}</p>
              <p className="text-[11px] text-muted-foreground">{commit.author} · {commit.date}</p>
            </div>
          </div>
        ))}
      </div>
    </ScrollArea>
  )
}
