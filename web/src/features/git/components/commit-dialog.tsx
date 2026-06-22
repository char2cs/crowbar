import { useEffect, useState } from 'react'
import { Dialog, DialogPopup, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Textarea } from '@/components/ui/textarea'
import { commitChanges } from '../api/git-commits-api'
import { stagePaths, unstagePaths } from '../api/git-status-api'
import type { GitFile } from '../types/git-types'

interface CommitDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  wsId: string
  files: GitFile[]
  /** Fired after a successful commit (refresh the section + diffs). */
  onCommitted: () => void
}

export function CommitDialog({ open, onOpenChange, wsId, files, onCommitted }: CommitDialogProps) {
  const [message, setMessage] = useState('')
  // Snapshot the file set when the dialog opens so a background git-status refresh
  // can't mutate the list (and therefore the committed set) while the user edits.
  const [dialogFiles, setDialogFiles] = useState<GitFile[]>(files)
  const [checked, setChecked] = useState<Set<string>>(() => new Set(files.map((f) => f.path)))
  const [isCommitting, setIsCommitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Reset to a fresh state (snapshot the files, all checked, empty message) on open.
  useEffect(() => {
    if (!open) return
    setDialogFiles(files)
    setChecked(new Set(files.map((f) => f.path)))
    setMessage('')
    setError(null)
    setIsCommitting(false)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

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
    const stage = dialogFiles.filter((f) => checked.has(f.path)).map((f) => f.path)
    const unstage = dialogFiles.filter((f) => !checked.has(f.path)).map((f) => f.path)
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
      onOpenChange(false)
      onCommitted()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to commit changes')
    } finally {
      setIsCommitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogPopup className="max-w-md">
        <DialogHeader>
          <DialogTitle>Commit changes</DialogTitle>
        </DialogHeader>
        <div className="flex flex-col gap-3 px-6">
          {error && (
            <div className="ui-text-xs rounded-md border border-destructive/30 bg-destructive/10 px-2.5 py-1.5 text-destructive">
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
          <div>
            <div className="ui-text-xs mb-1.5 text-muted-foreground">
              {dialogFiles.length} file{dialogFiles.length !== 1 ? 's' : ''}
            </div>
            <div className="flex max-h-48 flex-col gap-1 overflow-auto">
              {dialogFiles.map((f) => (
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
          </div>
        </div>
        <DialogFooter>
          <span className="ui-text-xs mr-auto self-center text-muted-foreground">⌘↵ to commit</span>
          <Button variant="outline" size="sm" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button variant="default" size="sm" disabled={!canCommit} onClick={() => void handleCommit()}>
            {isCommitting ? 'Committing…' : 'Commit'}
          </Button>
        </DialogFooter>
      </DialogPopup>
    </Dialog>
  )
}
