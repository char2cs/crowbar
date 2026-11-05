// web/src/components/projects/ImportProjectModal.tsx
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
import { openNativeDialog as openDialog } from '@/lib/native-dialog'
import { isTauri } from '@/lib/crowbar-bridge'
import { createProject } from '@/lib/store/create-project'
import { LOCAL_HOST } from '@/features/hosts/types'
import type { Project } from '@/lib/types'
import { report } from '@/lib/errors/report'

interface ImportProjectModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onImport: (project: Project) => void
}

// The daemon imports from an absolute path on its own filesystem, so the path is typed: a browser
// folder picker (webkitdirectory) yields only the folder name.
export function ImportProjectModal({ open, onOpenChange, onImport }: ImportProjectModalProps) {
  const [selectedPath, setSelectedPath] = useState('')
  const [projectName, setProjectName] = useState('')
  const [loading, setLoading] = useState(false)

  const trimmedPath = selectedPath.trim()
  const pathLooksAbsolute = trimmedPath.startsWith('/') || /^[A-Za-z]:[\\/]/.test(trimmedPath)

  async function handleBrowse() {
    const selected = await openDialog({ directory: true, multiple: false })
    if (typeof selected === 'string') setSelectedPath(selected)
  }

  const handleImport = async () => {
    if (!pathLooksAbsolute) return
    setLoading(true)
    try {
      const fallbackName =
        trimmedPath
          .replace(/[\\/]+$/, '')
          .split(/[\\/]/)
          .pop() ?? trimmedPath
      const name = projectName.trim() || fallbackName
      // The path is picked on this machine, so the local daemon imports it.
      onImport(await createProject(name, trimmedPath, LOCAL_HOST.id))
      setSelectedPath('')
      setProjectName('')
    } catch (err) {
      report(err, { action: 'Failed to import project' })
    } finally {
      setLoading(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Import project</DialogTitle>
        </DialogHeader>

        <div className="space-y-5 px-6 py-5">
          <div className="space-y-2">
            <label htmlFor="import-project-path" className="text-sm font-medium text-foreground">
              Project folder
            </label>
            <div className="flex gap-2">
              <Input
                id="import-project-path"
                value={selectedPath}
                onChange={(e) => setSelectedPath(e.target.value)}
                placeholder="/absolute/path/to/project"
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
            <label htmlFor="import-project-name" className="text-sm font-medium text-foreground">
              Project name
            </label>
            <Input
              id="import-project-name"
              value={projectName}
              onChange={(e) => setProjectName(e.target.value)}
              placeholder="My project"
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={handleImport} disabled={!pathLooksAbsolute || loading}>
            {loading ? 'Importing…' : 'Import'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
