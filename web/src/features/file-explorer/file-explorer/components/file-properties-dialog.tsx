import { Info } from '@phosphor-icons/react'
import { AppDialog as Dialog } from '@/components/ui/dialog'
import type { FileProperties } from '@/features/file-explorer/file-explorer/lib/file-properties'

export function FilePropertiesDialog({
  properties,
  onClose,
}: {
  properties: FileProperties
  onClose: () => void
}) {
  return (
    <Dialog title="Properties" icon={Info} onClose={onClose} size="md">
      <dl className="grid grid-cols-[72px_1fr] gap-x-3 gap-y-2 ui-font ui-text-sm">
        <dt className="text-muted-foreground">File</dt>
        <dd className="min-w-0 break-words text-foreground">{properties.fileName}</dd>
        <dt className="text-muted-foreground">Path</dt>
        <dd className="min-w-0 break-words text-foreground">{properties.path}</dd>
        <dt className="text-muted-foreground">Size</dt>
        <dd className="text-foreground">{properties.size}</dd>
        <dt className="text-muted-foreground">Type</dt>
        <dd className="text-foreground">{properties.type}</dd>
      </dl>
    </Dialog>
  )
}
