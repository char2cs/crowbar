import { WarningCircle } from '@phosphor-icons/react'
import { useStore } from 'zustand'
import { Button } from '@/components/ui/button'
import { entityStore, type EntityState } from '@/core/entities/entity-store'
import { selectRepos, useSelect } from '@/core/entities/select'
import { resubscribe } from '@/core/stream/stream'

const NONE: readonly string[] = []

const selectUnread = (s: EntityState) => s.unread.projects ?? NONE
/** A sub clears the scope's synced and its snapshot's synced sets it again. */
const selectAwaiting = (s: EntityState) => !s.synced.projects

interface Line {
  key: string
  text: string
}

/** What the projects snapshot could not read of `projectId`: its home's rows, or a repo's.
 *  Retry asks for the snapshot again, which reads them anew. */
export function UnreadNotice({ projectId }: { projectId: string }) {
  const unread = useStore(entityStore, selectUnread)
  const awaiting = useStore(entityStore, selectAwaiting)
  const repos = useSelect(selectRepos)
  if (unread.length === 0) return null
  const ids = new Set(unread)
  const lines: Line[] = []
  if (ids.has(projectId)) lines.push({ key: projectId, text: "Couldn't load this project's chats" })
  for (const repo of repos) {
    if (repo.projectId === projectId && ids.has(repo.id)) {
      lines.push({ key: repo.id, text: `Couldn't load chats in ${repo.name}` })
    }
  }
  if (lines.length === 0) return null
  return (
    <div role="alert" className="flex items-start gap-2 px-3 py-1.5 text-xs text-muted-foreground">
      {/* text-warning alone is 2.2:1 on the light sidebar, under the 3:1 an icon needs. */}
      <WarningCircle
        className="mt-0.5 size-3.5 shrink-0 text-warning-foreground dark:text-warning"
        aria-hidden
      />
      <div className="flex min-w-0 flex-1 flex-col">
        {lines.map((line) => (
          <span key={line.key} className="break-words">
            {line.text}
          </span>
        ))}
      </div>
      <Button variant="ghost" size="xs" disabled={awaiting} onClick={() => resubscribe('projects')}>
        Retry
      </Button>
    </div>
  )
}
