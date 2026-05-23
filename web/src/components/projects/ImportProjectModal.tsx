// web/src/components/projects/ImportProjectModal.tsx
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import type { Project } from '@/lib/types'

interface ImportProjectModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onImport: (project: Project) => void
}

export function ImportProjectModal({ open, onOpenChange }: ImportProjectModalProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Import project</DialogTitle>
        </DialogHeader>
        <p className="text-sm text-muted-foreground">Coming soon…</p>
      </DialogContent>
    </Dialog>
  )
}
