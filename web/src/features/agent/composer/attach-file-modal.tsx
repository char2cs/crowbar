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

/**
 * A single entry point regardless of kind — image, CSV, PDF, other — the
 * kind is inferred from the uploaded file's own contentType, not from
 * anything the user picked here.
 *
 * Modal, not inline: the composer pill has no room to grow a picker without
 * shoving the transcript around mid-drag.
 */
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

  // Upload, CSV-inline resolution, and the failure toast all live in the
  // shared hook now (also used by agent-composer.tsx and
  // agent-empty-document.tsx) — this wrapper only adds the `uploading` state
  // local to this modal, in a `finally` around it. `handleInserted` only
  // fires on success, so `onClose` still never runs on a failed upload.
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

  // Memoized for the same reason agent-composer.tsx's own pill drop handler
  // is (Task 29, same hook): `useTauriFileDrop`'s effect depends on
  // `[containerRef, onDrop]` and re-subscribes Tauri's `onDragDropEvent` IPC
  // channel whenever `onDrop` gets a new identity. Here that would mean every
  // `dragOver`/`dragLeave` re-render (each toggles `dropTarget`) tearing the
  // subscription down and back up continuously for the whole span of an
  // active drag — an inline arrow would churn it far more than Task 29's
  // per-keystroke case did.
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

  // Plain-browser fallback — a real DataTransfer.files DOES carry usable File
  // bytes here, unrelated to extractDroppedFilePaths (which is about a host
  // PATH, not bytes).
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
