// web/src/components/projects/ImportProjectModal.tsx
import { useRef, useState } from 'react'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { toast } from 'sonner'
import { postProject, fetchProject } from '@/lib/api'
import type { Project } from '@/lib/types'

interface ImportProjectModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onImport: (project: Project) => void
}

export function ImportProjectModal({ open, onOpenChange, onImport }: ImportProjectModalProps) {
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [selectedPath, setSelectedPath] = useState('')
  const [projectName, setProjectName] = useState('')
  const [loading, setLoading] = useState(false)

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    // webkitRelativePath is "folderName/..." — take the first segment
    const folderName = file.webkitRelativePath.split('/')[0] || file.name
    setSelectedPath(folderName)
    setProjectName(prev => prev || folderName)
  }

  const handleImport = async () => {
    if (!selectedPath) return
    setLoading(true)
    try {
      // The mutation returns only { id }; re-fetch the full project so the
      // sidebar gets a complete entity (name/path) rather than undefined fields.
      const { id } = await postProject(projectName || selectedPath, selectedPath)
      const project = await fetchProject(id)
      onImport(project)
      setSelectedPath('')
      setProjectName('')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to import project')
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

        <div className="space-y-4 py-2">
          {/* Hidden OS folder picker */}
          <input
            ref={fileInputRef}
            type="file"
            // @ts-ignore — webkitdirectory is not in React types
            webkitdirectory=""
            className="hidden"
            onChange={handleFileChange}
          />

          <div className="space-y-1.5">
            <label className="text-sm font-medium text-foreground">Project folder</label>
            <div className="flex gap-2">
              <Input
                readOnly
                value={selectedPath}
                placeholder="No folder selected"
                className="flex-1 font-mono text-[12px]"
              />
              <Button
                variant="outline"
                size="sm"
                type="button"
                onClick={() => fileInputRef.current?.click()}
              >
                Choose…
              </Button>
            </div>
          </div>

          <div className="space-y-1.5">
            <label className="text-sm font-medium text-foreground">Project name</label>
            <Input
              value={projectName}
              onChange={e => setProjectName(e.target.value)}
              placeholder="My project"
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button onClick={handleImport} disabled={!selectedPath || loading}>
            {loading ? 'Importing…' : 'Import'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
