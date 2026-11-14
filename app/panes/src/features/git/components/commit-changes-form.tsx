import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Textarea } from '@/components/ui/textarea'
import { commitFiles } from '../api/git-commits-api'
import { requestGitRefresh } from '../stores/git-refresh'
import type { GitFile } from '../types/git-types'

/** Commits the shown changes of `wsId` the person leaves ticked, so a branch the merge box
 *  blocks on "commit them first" can be committed where it says so, and nothing unseen rides along. */
export function CommitChangesForm({ wsId, files }: { wsId: string; files: GitFile[] }) {
  const [message, setMessage] = useState('')
  const [committing, setCommitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  // The unticked, so a file that appears later is shown ticked like the rest.
  const [left, setLeft] = useState<ReadonlySet<string>>(new Set())
  const ticked = files.filter((file) => !left.has(file.path))
  const canCommit = message.trim().length > 0 && ticked.length > 0 && !committing

  const tick = (path: string, on: boolean) =>
    setLeft((was) => {
      const next = new Set(was)
      if (on) next.delete(path)
      else next.add(path)
      return next
    })

  const commit = async () => {
    if (!canCommit) return
    setCommitting(true)
    setError(null)
    try {
      const out = files.filter((file) => left.has(file.path) && file.staged)
      await commitFiles(
        wsId,
        message,
        ticked.map((file) => file.path),
        out.map((file) => file.path),
      )
      setMessage('')
      requestGitRefresh(wsId)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setCommitting(false)
    }
  }

  return (
    <div className="flex min-w-0 flex-col gap-2">
      <ul className="flex max-h-40 min-w-0 flex-col gap-1 overflow-y-auto">
        {files.map((file) => (
          <li key={file.path} className="ui-text-xs flex min-w-0 items-center gap-2">
            <Checkbox
              ariaLabel={file.path}
              checked={!left.has(file.path)}
              disabled={committing}
              onChange={(on: boolean) => tick(file.path, on)}
            />
            <span className="min-w-0 truncate" title={file.path}>
              {file.path}
            </span>
          </li>
        ))}
      </ul>
      <Textarea
        aria-label="Commit message"
        value={message}
        disabled={committing}
        onChange={(e) => setMessage(e.target.value)}
        onKeyDown={(e) => {
          if (e.key !== 'Enter' || !(e.metaKey || e.ctrlKey)) return
          e.preventDefault()
          void commit()
        }}
        placeholder="Describe the change…"
        className="ui-text-sm min-h-16 resize-none"
      />
      <Button size="sm" className="w-full" disabled={!canCommit} onClick={() => void commit()}>
        {committing
          ? 'Committing…'
          : `Commit ${ticked.length} change${ticked.length === 1 ? '' : 's'}`}
      </Button>
      {error && <p className="ui-text-xs text-destructive">{error}</p>}
    </div>
  )
}
