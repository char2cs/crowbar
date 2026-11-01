import type { ChangeEvent, DragEvent } from 'react'
import { useCallback, useRef, useState } from 'react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogDescription,
  DialogHeader,
  DialogPopup,
  DialogTitle,
} from '@/components/ui/dialog'
import type { UploadChatAttachmentInput } from '@/features/agent/api/upload-chat-attachment'
import { useAttachmentUpload } from '@/features/agent/composer/lib/use-attachment-upload'
import { useTauriFileDrop } from '@/features/file-system/lib/tauri-file-drop'
import { cn } from '@/lib/utils'

interface AttachFileModalProps {
  wsId: string
  chatId: string
  open: boolean
  onClose: () => void
  onInsertMarkdown: (markdown: string) => void
}

/** Modal, not inline: the composer pill has no room to grow a picker without
 *  shoving the transcript around mid-drag. */
export function AttachFileModal({
  wsId,
  chatId,
  open,
  onClose,
  onInsertMarkdown,
}: AttachFileModalProps) {
  const [dropTarget, setDropTarget] = useState(false)
  const [uploading, setUploading] = useState(false)
  const dropzoneRef = useRef<HTMLDivElement>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const handleInserted = useCallback(
    (markdown: string) => {
      onInsertMarkdown(markdown)
      onClose()
    },
    [onInsertMarkdown, onClose],
  )

  // `handleInserted` fires only on success, so a failed upload keeps the modal open.
  const { uploadAndInsert: coreUploadAndInsert } = useAttachmentUpload(wsId, chatId, handleInserted)

  const uploadAndInsert = useCallback(
    async (input: UploadChatAttachmentInput) => {
      setUploading(true)
      try {
        await coreUploadAndInsert(input)
      } finally {
        setUploading(false)
      }
    },
    [coreUploadAndInsert],
  )

  // Memoized: `useTauriFileDrop` re-subscribes its IPC channel on a new `onDrop`,
  // and every dragOver/dragLeave re-renders this modal.
  const handleTauriDrop = useCallback(
    (paths: string[]) => {
      setDropTarget(false)
      for (const path of paths) void uploadAndInsert({ path })
    },
    [uploadAndInsert],
  )

  useTauriFileDrop(dropzoneRef, handleTauriDrop)

  const handleDragOver = useCallback((e: DragEvent) => {
    if (!e.dataTransfer.types.includes('Files')) return
    e.preventDefault()
    setDropTarget(true)
  }, [])

  const handleDragLeave = useCallback(() => setDropTarget(false), [])

  // Plain-browser fallback: File bytes, never a host path.
  const handleDrop = useCallback(
    (e: DragEvent) => {
      setDropTarget(false)
      if (!e.dataTransfer.types.includes('Files')) return
      e.preventDefault()
      for (const file of Array.from(e.dataTransfer.files)) void uploadAndInsert({ file })
    },
    [uploadAndInsert],
  )

  const handleBrowse = useCallback(
    (e: ChangeEvent<HTMLInputElement>) => {
      for (const file of Array.from(e.target.files ?? [])) void uploadAndInsert({ file })
      e.target.value = ''
    },
    [uploadAndInsert],
  )

  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogPopup className="max-w-md">
        <DialogHeader>
          <DialogTitle>Attach a file</DialogTitle>
          <DialogDescription>Drop it here, or choose one from your computer.</DialogDescription>
        </DialogHeader>
        <div
          ref={dropzoneRef}
          data-testid="attach-file-dropzone"
          className={cn(
            'flex flex-col items-center gap-3 rounded-xl border-2 border-dashed border-input p-8 text-center text-muted-foreground transition-colors',
            dropTarget && 'border-ring bg-accent/50 text-foreground',
          )}
          onDragOver={handleDragOver}
          onDragLeave={handleDragLeave}
          onDrop={handleDrop}
        >
          <p>{uploading ? 'Uploading…' : 'Drag a file here'}</p>
          <Button
            type="button"
            variant="outline"
            disabled={uploading}
            onClick={() => fileInputRef.current?.click()}
          >
            Choose a file
          </Button>
          <input
            ref={fileInputRef}
            type="file"
            aria-label="Choose a file"
            className="sr-only"
            disabled={uploading}
            onChange={handleBrowse}
          />
        </div>
      </DialogPopup>
    </Dialog>
  )
}
