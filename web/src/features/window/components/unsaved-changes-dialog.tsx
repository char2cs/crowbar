import { Warning } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { AppDialog } from '@/components/ui/dialog'

interface UnsavedChangesDialogProps {
  fileName?: string
  onSave?: () => void
  onDiscard?: () => void
  onCancel?: () => void
}

export function UnsavedChangesDialog({ fileName, onSave, onDiscard, onCancel }: UnsavedChangesDialogProps) {
  return (
    <AppDialog
      title="Unsaved Changes"
      icon={Warning}
      onClose={onCancel}
      size="sm"
      footer={
        <>
          <Button variant="ghost" compact onClick={onCancel}>
            Cancel
          </Button>
          <Button variant="destructive" compact onClick={onDiscard}>
            Discard
          </Button>
          <Button variant="default" compact onClick={onSave}>
            Save
          </Button>
        </>
      }
    >
      <p className="text-sm text-foreground">
        {fileName
          ? <><span className="font-medium">{fileName}</span> has unsaved changes.</>
          : 'This file has unsaved changes.'}
        {' '}Do you want to save before closing?
      </p>
    </AppDialog>
  )
}

export default UnsavedChangesDialog
