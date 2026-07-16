import type { ReactElement } from 'react'
import { useState } from 'react'
import { Popover, PopoverTrigger, PopoverContent, PopoverTitle } from '@/components/ui/popover'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Textarea } from '@/components/ui/textarea'
import { commitChanges } from '../api/git-commits-api'
import { stagePaths, unstagePaths } from '../api/git-status-api'
import type { GitFile } from '../types/git-types'

interface CommitPopoverProps {
  wsId: string
  files: GitFile[]
  /** Fired after a successful commit (refresh the section + diffs). */
  onCommitted: () => void
  /** The "Commit changes" button rendered as the popover trigger. */
  trigger: ReactElement
}

// Accepted (prefer-useReducer): these five states only "change together" on the
// once-per-open reset; every other update (typing a message, toggling a file,
// commit progress/error) touches exactly one of them. A reducer would centralize
// five independent fields for the sake of one batched reset — style call, judged
// not worth the churn (task-21 batch 3).
// react-doctor-disable-next-line prefer-useReducer
export function CommitPopover({ wsId, files, onCommitted, trigger }: CommitPopoverProps) {
  const [open, setOpen] = useState(false)
  const [message, setMessage] = useState('')
  // Snapshot the file set when the popover opens so a background git-status refresh
  // can't mutate the list (and therefore the committed set) while the user edits.
  const [popoverFiles, setPopoverFiles] = useState<GitFile[]>(files)
  const [checked, setChecked] = useState<Set<string>>(() => new Set(files.map((f) => f.path)))
  const [isCommitting, setIsCommitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Reset to a fresh state (snapshot the files, all checked, empty message) when
  // the popover opens. Done in the open handler — not a useEffect reacting to
  // `open` — so the writes happen in the event that triggers them (no derived-
  // state chain) and `popoverFiles` stays a true at-open SNAPSHOT: later `files`
  // prop updates from a background git-status refresh can't flow in mid-edit.
  const handleOpenChange = (nextOpen: boolean) => {
    if (nextOpen) {
      setPopoverFiles(files)
      setChecked(new Set(files.map((f) => f.path)))
      setMessage('')
      setError(null)
      setIsCommitting(false)
    }
    setOpen(nextOpen)
  }

  const canCommit = message.trim().length > 0 && checked.size > 0 && !isCommitting

  const toggle = (path: string) =>
    setChecked((prev) => {
      const next = new Set(prev)
      if (next.has(path)) next.delete(path)
      else next.add(path)
      return next
    })

  const handleCommit = async () => {
    if (!canCommit) return
    setIsCommitting(true)
    setError(null)
    const stage: string[] = []
    const unstage: string[] = []
    for (const f of popoverFiles) {
      ;(checked.has(f.path) ? stage : unstage).push(f.path)
    }
    try {
      // stagePaths/unstagePaths swallow their errors and return false (they toast),
      // so guard the booleans — never commit against a half-staged index.
      if (stage.length && !(await stagePaths(wsId, stage))) {
        setError('Failed to stage changes')
        return
      }
      if (unstage.length && !(await unstagePaths(wsId, unstage))) {
        setError('Failed to update staging')
        return
      }
      const ok = await commitChanges(wsId, message.trim())
      if (!ok) {
        setError('Failed to commit changes')
        return
      }
      setOpen(false)
      onCommitted()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to commit changes')
    } finally {
      setIsCommitting(false)
    }
  }

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <PopoverTrigger render={trigger} />
      <PopoverContent side="top" align="start" className="w-80">
        <PopoverTitle className="mb-2 text-sm">Commit changes</PopoverTitle>
        {error && (
          <div className="ui-text-xs mb-2 rounded-md border border-destructive/30 bg-destructive/10 px-2.5 py-1.5 text-destructive">
            {error}
          </div>
        )}
        <Textarea
          autoFocus
          value={message}
          disabled={isCommitting}
          onChange={(e) => setMessage(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
              e.preventDefault()
              void handleCommit()
            }
          }}
          placeholder="Commit message…"
          className="ui-font ui-text-sm min-h-20 resize-none"
        />
        <div className="ui-text-xs mt-2 mb-1.5 text-muted-foreground">
          {popoverFiles.length} file{popoverFiles.length !== 1 ? 's' : ''}
        </div>
        <div className="flex max-h-40 flex-col gap-1 overflow-auto">
          {popoverFiles.map((f) => (
            <label key={f.path} className="ui-text-sm flex cursor-pointer items-center gap-2">
              <Checkbox
                checked={checked.has(f.path)}
                onChange={() => toggle(f.path)}
                disabled={isCommitting}
              />
              <span className="truncate">{f.path}</span>
            </label>
          ))}
        </div>
        <div className="mt-3 flex items-center justify-between gap-2">
          <span className="ui-text-xs text-muted-foreground">⌘↵ to commit</span>
          <div className="flex gap-2">
            <Button variant="outline" size="sm" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button
              variant="default"
              size="sm"
              disabled={!canCommit}
              onClick={() => void handleCommit()}
            >
              {isCommitting ? 'Committing…' : 'Commit'}
            </Button>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  )
}
