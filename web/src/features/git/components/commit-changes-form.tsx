import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { commitAll } from '../api/git-commits-api'
import { requestGitRefresh } from '../stores/git-refresh'

/** Commits every uncommitted change of `wsId`, so a branch the merge box blocks on "commit them
 *  first" can be committed where it says so. */
export function CommitChangesForm({ wsId, changes }: { wsId: string; changes: number }) {
  const [message, setMessage] = useState('')
  const [committing, setCommitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const canCommit = message.trim().length > 0 && !committing

  const commit = async () => {
    if (!canCommit) return
    setCommitting(true)
    setError(null)
    try {
      await commitAll(wsId, message)
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
        {committing ? 'Committing…' : `Commit ${changes} change${changes === 1 ? '' : 's'}`}
      </Button>
      {error && <p className="ui-text-xs text-destructive">{error}</p>}
    </div>
  )
}
