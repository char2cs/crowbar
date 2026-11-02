import { WarningCircle } from '@phosphor-icons/react'
import { useStore } from 'zustand'
import { Button } from '@/components/ui/button'
import { entityStore, type EntityState } from '@/core/entities/entity-store'
import { selectRepos, useSelect } from '@/core/entities/select'
import { resubscribe } from '@/core/stream/stream'

const NONE: readonly string[] = []

const selectUnread = (s: EntityState) => s.unread.projects ?? NONE

/** What the projects snapshot could not read of `projectId`: its home's rows, or a repo's.
 *  Retry asks for the snapshot again, which reads them anew. */
export function UnreadNotice({ projectId }: { projectId: string }) {
  const unread = useStore(entityStore, selectUnread)
  const repos = useSelect(selectRepos)
  if (unread.length === 0) return null
  const ids = new Set(unread)
  const lines: string[] = []
  if (ids.has(projectId)) lines.push("Couldn't load this project's chats")
  for (const repo of repos) {
    if (repo.projectId === projectId && ids.has(repo.id)) {
      lines.push(`Couldn't load chats in ${repo.name}`)
    }
  }
  if (lines.length === 0) return null
  return (
    <div role="alert" className="flex items-start gap-2 px-3 py-1.5 text-xs text-muted-foreground">
      <WarningCircle className="mt-0.5 size-3.5 shrink-0" aria-hidden />
      <div className="flex min-w-0 flex-1 flex-col">
        {lines.map((line) => (
          <span key={line} className="truncate">
            {line}
          </span>
        ))}
      </div>
      <Button variant="ghost" size="xs" onClick={() => resubscribe('projects')}>
        Retry
      </Button>
    </div>
  )
}
