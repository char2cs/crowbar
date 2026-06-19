import { useState } from 'react'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { FolderOpen } from '@phosphor-icons/react'
import { toast } from 'sonner'
import { open as openDialog } from '@tauri-apps/plugin-dialog'
import { isTauri } from '@/lib/crowbar-bridge'
import { postRepo } from '@/lib/api'
import { useProjectStore } from '@/lib/store/projects'

interface AddRepositoryModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function AddRepositoryModal({ open, onOpenChange }: AddRepositoryModalProps) {
  const [path, setPath] = useState('')
  const [name, setName] = useState('')
  const [loading, setLoading] = useState(false)

  const trimmedPath = path.trim()
  const pathLooksAbsolute = trimmedPath.startsWith('/') || /^[A-Za-z]:[\\/]/.test(trimmedPath)

  async function handleBrowse() {
    const selected = await openDialog({ directory: true, multiple: false })
    if (typeof selected === 'string') {
      setPath(selected)
      if (!name) {
        setName(selected.replace(/[\\/]+$/, '').split(/[\\/]/).pop() ?? '')
      }
    }
  }

  async function handleAdd() {
    if (!pathLooksAbsolute) return
    setLoading(true)
    try {
      const activeProjectId = useProjectStore.getState().activeProjectId
      const fallbackName =
        trimmedPath
          .replace(/[\\/]+$/, '')
          .split(/[\\/]/)
          .pop() ?? trimmedPath
      const repoName = name.trim() || fallbackName

      // §3: postRepo is 202 with no body. The daemon imports the repo (and its
      // default-branch workspace) and broadcasts the RepoDTO/WorkspaceDTO over
      // the §7 entity streams, which seed the sidebar cache. Resolving the new
      // repo/ws ids from those WS frames to navigate is W18; for now we just
      // fire the mutation and close the modal.
      await postRepo(activeProjectId, repoName, trimmedPath)

      onOpenChange(false)
      setPath('')
      setName('')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to add repository')
    } finally {
      setLoading(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Add repository</DialogTitle>
        </DialogHeader>

        <div className="space-y-5 px-6 py-5">
          <div className="space-y-2">
            <label className="text-sm font-medium text-foreground">Repository folder</label>
            <div className="flex gap-2">
              <Input
                value={path}
                onChange={(e) => setPath(e.target.value)}
                placeholder="/absolute/path/to/repo"
                className="font-mono text-[12px]"
              />
              {isTauri() && (
                <Button variant="outline" size="icon" onClick={handleBrowse} title="Browse…">
                  <FolderOpen className="size-4" />
                </Button>
              )}
            </div>
            {trimmedPath !== '' && !pathLooksAbsolute && (
              <p className="text-[12px] text-destructive">
                Enter an absolute path (e.g. /Users/you/code/my-repo)
              </p>
            )}
          </div>

          <div className="space-y-2">
            <label className="text-sm font-medium text-foreground">
              Repository name{' '}
              <span className="font-normal text-muted-foreground">(optional)</span>
            </label>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Derived from folder name"
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={handleAdd} disabled={!pathLooksAbsolute || loading}>
            {loading ? 'Adding…' : 'Add repository'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
