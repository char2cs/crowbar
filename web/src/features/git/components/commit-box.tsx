import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { commitChanges } from '../api/git-commits-api'
import { stagePaths } from '../api/git-status-api'
import type { GitFile } from '../types/git-types'

interface CommitBoxProps {
  wsId: string
  files: GitFile[]
  /** Fired after a successful commit (refresh the section + diffs). */
  onCommitted: () => void
}

// Spec 6.3: the commit box is inline and always visible — no popover. Commit
// stages every currently-changed file, then commits; there is no per-file
// staging picker here, since the changed list below the fold is already that
// list (spec: "the card already is that list").
//
// The "Pull request" action has no backend yet (git-remotes-api's remote
// listing is still a stub, and there is no PR-creation endpoint) — it stays a
// disabled placeholder rather than a fake network call. It is still rendered,
// per spec, directly beside Commit.
export function CommitBox({ wsId, files, onCommitted }: CommitBoxProps) {
  const [message, setMessage] = useState('')
  const [isCommitting, setIsCommitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const canCommit = message.trim().length > 0 && files.length > 0 && !isCommitting

  const handleCommit = async () => {
    if (!canCommit) return
    setIsCommitting(true)
    setError(null)
    try {
      const paths = files.map((f) => f.path)
      if (paths.length && !(await stagePaths(wsId, paths))) {
        setError('Failed to stage changes')
        return
      }
      const ok = await commitChanges(wsId, message.trim())
      if (!ok) {
        setError('Failed to commit changes')
        return
      }
      setMessage('')
      onCommitted()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to commit changes')
    } finally {
      setIsCommitting(false)
    }
  }

  return (
    <div className="flex flex-col gap-2 p-3" aria-label="Commit">
      {error && (
        <div className="ui-text-xs rounded-md border border-destructive/30 bg-destructive/10 px-2.5 py-1.5 text-destructive">
          {error}
        </div>
      )}
      <Textarea
        value={message}
        disabled={isCommitting}
        onChange={(e) => setMessage(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
            e.preventDefault()
            void handleCommit()
          }
        }}
        placeholder="Describe the change…"
        className="ui-font ui-text-sm min-h-16 resize-none"
      />
      <div className="flex items-center gap-2">
        <Button
          variant="default"
          size="sm"
          className="flex-1"
          disabled={!canCommit}
          onClick={() => void handleCommit()}
        >
          {isCommitting ? 'Committing…' : 'Commit'}
        </Button>
        <Button
          variant="outline"
          size="sm"
          className="flex-1"
          disabled
          tooltip="Pull request creation isn't wired up yet — push your branch, then open one on your git host."
        >
          Pull request
        </Button>
      </div>
    </div>
  )
}
