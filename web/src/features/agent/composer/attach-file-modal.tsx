import type { ChangeEvent, DragEvent, KeyboardEvent } from 'react'
import { useCallback, useRef, useState } from 'react'
import { XIcon } from '@phosphor-icons/react'
import { Dialog, DialogPopup, DialogTitle } from '@/components/ui/dialog'
import type { UploadChatAttachmentInput } from '@/features/agent/api/upload-chat-attachment'
import { uploadAttachmentMarkdown } from '@/features/agent/composer/lib/attachment-upload'
import { useTauriFileDrop } from '@/features/file-system/lib/tauri-file-drop'
import { UNREACHED, userWords } from '@/lib/errors/report'
import { IS_MOBILE } from '@/utils/platform'

/** The daemon's attachments.MaxBytes (see upload-chat-attachment.ts). */
const LIMIT_MB = 25

type Status = 'uploading' | 'done' | 'bad'

interface Staged {
  id: number
  name: string
  /** Unknown for a path the desktop hands over: the daemon reads it. */
  bytes: number | null
  status: Status
  /** Bytes sent so far; stays 0 for a path the daemon reads itself. */
  loaded?: number
  markdown?: string
  problem?: string
}

interface AttachFileModalProps {
  wsId: string
  chatId: string
  open: boolean
  onClose: () => void
  onInsertMarkdown: (markdown: string) => void
}

function sizeLabel(bytes: number | null): string {
  if (bytes === null) return ''
  if (bytes < 1024 * 1024) return `${Math.max(1, Math.round(bytes / 1024))} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1).replace(/\.0$/, '')} MB`
}

function rowNote(item: Staged): string {
  const size = sizeLabel(item.bytes)
  if (item.status === 'uploading' && item.bytes) {
    return `${sizeLabel(item.loaded ?? 0)} of ${size} · Uploading…`
  }
  const note =
    item.status === 'done' ? 'Uploaded' : item.status === 'uploading' ? 'Uploading…' : item.problem
  return [size, note].filter(Boolean).join(' · ')
}

const baseName = (path: string) => path.split(/[\\/]/).pop() ?? path

/** Files are uploaded as they are chosen and listed with their state; "Attach" puts the finished
 *  ones into the message. A modal, not inline: the pill has no room to grow a picker. */
export function AttachFileModal({
  wsId,
  chatId,
  open,
  onClose,
  onInsertMarkdown,
}: AttachFileModalProps) {
  const [items, setItems] = useState<Staged[]>([])
  const [dropTarget, setDropTarget] = useState(false)
  const dropzoneRef = useRef<HTMLDivElement>(null)
  const filesRef = useRef<HTMLInputElement>(null)
  const photosRef = useRef<HTMLInputElement>(null)
  const cameraRef = useRef<HTMLInputElement>(null)
  const nextId = useRef(0)
  const uploads = useRef(new Map<number, AbortController>())

  const settle = useCallback((id: number, patch: Partial<Staged>) => {
    // A removed row's upload may still land: it finds nothing to update.
    setItems((current) => current.map((item) => (item.id === id ? { ...item, ...patch } : item)))
  }, [])

  const stage = useCallback(
    (input: UploadChatAttachmentInput) => {
      const id = nextId.current++
      const name = 'file' in input ? input.file.name : baseName(input.path)
      const bytes = 'file' in input ? input.file.size : null
      if (bytes !== null && bytes > LIMIT_MB * 1024 * 1024) {
        const problem = `too large, the limit is ${LIMIT_MB} MB`
        setItems((current) => [...current, { id, name, bytes, status: 'bad', problem }])
        return
      }
      setItems((current) => [...current, { id, name, bytes, status: 'uploading' }])
      const controller = new AbortController()
      uploads.current.set(id, controller)
      uploadAttachmentMarkdown(wsId, chatId, input, {
        signal: controller.signal,
        onProgress: (loaded) => settle(id, { loaded }),
      }).then(
        (markdown) => settle(id, { status: 'done', markdown }),
        (err: unknown) => {
          if (controller.signal.aborted) return
          settle(id, { status: 'bad', problem: userWords(err, UNREACHED) ?? UNREACHED })
        },
      )
    },
    [wsId, chatId, settle],
  )

  // Memoized: `useTauriFileDrop` re-subscribes its IPC channel on a new `onDrop`,
  // and every dragOver/dragLeave re-renders this modal.
  const handleTauriDrop = useCallback(
    (paths: string[]) => {
      setDropTarget(false)
      for (const path of paths) stage({ path })
    },
    [stage],
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
      for (const file of Array.from(e.dataTransfer.files)) stage({ file })
    },
    [stage],
  )

  const handleBrowse = (e: ChangeEvent<HTMLInputElement>) => {
    for (const file of Array.from(e.target.files ?? [])) stage({ file })
    e.target.value = ''
  }

  const done = items.filter((item) => item.status === 'done')
  const worth = items.filter((item) => item.status !== 'bad')
  const uploading = worth.length > done.length
  const ready = done.length > 0 && !uploading

  const attachLabel =
    worth.length === 0
      ? 'Attach files'
      : `Attach ${worth.length} ${worth.length === 1 ? 'file' : 'files'}`

  const attach = () => {
    if (!ready) return
    for (const item of done) if (item.markdown) onInsertMarkdown(item.markdown)
    onClose()
  }

  const handleKeyDown = (e: KeyboardEvent) => {
    if (e.key !== 'Enter' || e.target instanceof HTMLButtonElement) return
    e.preventDefault()
    attach()
  }

  const remove = (id: number) => {
    uploads.current.get(id)?.abort()
    uploads.current.delete(id)
    setItems((current) => current.filter((item) => item.id !== id))
  }

  // Closing without attaching abandons what is still on its way.
  const cancel = () => {
    for (const controller of uploads.current.values()) controller.abort()
    uploads.current.clear()
    onClose()
  }

  const chosen = (
    <>
      {items.length > 0 && (
        <div className="attach-files" data-testid="attach-file-list">
          {items.map((item) => (
            <div key={item.id} className="attach-file" data-status={item.status}>
              <div className="attach-file-text">
                <div className="attach-file-name">{item.name}</div>
                <div className="attach-file-note">{rowNote(item)}</div>
                {item.status === 'uploading' && (
                  <div
                    className="attach-bar"
                    role="progressbar"
                    aria-label={`Uploading ${item.name}`}
                    aria-valuenow={
                      item.bytes ? Math.round(((item.loaded ?? 0) / item.bytes) * 100) : undefined
                    }
                    data-determinate={item.bytes ? '' : undefined}
                  >
                    <i
                      style={
                        item.bytes
                          ? { width: `${Math.round(((item.loaded ?? 0) / item.bytes) * 100)}%` }
                          : undefined
                      }
                    />
                  </div>
                )}
              </div>
              <button
                type="button"
                className="attach-x"
                aria-label={`Remove ${item.name}`}
                onClick={() => remove(item.id)}
              >
                <XIcon size={13} />
              </button>
            </div>
          ))}
        </div>
      )}
      <div className="attach-foot">
        {uploading && (
          <span className="attach-hint">
            Uploading {done.length} of {worth.length}…
          </span>
        )}
        <span className="attach-grow" />
        <button type="button" className="attach-btn" onClick={cancel}>
          Cancel
          {!IS_MOBILE && <kbd>esc</kbd>}
        </button>
        <button
          type="button"
          className="attach-btn"
          data-primary={ready || undefined}
          disabled={!ready}
          onClick={attach}
        >
          {attachLabel}
          {!IS_MOBILE && <kbd>↵</kbd>}
        </button>
      </div>
    </>
  )

  return (
    <Dialog open={open} onOpenChange={(next) => !next && cancel()}>
      <DialogPopup
        className={IS_MOBILE ? 'attach-sheet' : 'attach-dlg'}
        showCloseButton={false}
        onKeyDown={handleKeyDown}
      >
        {IS_MOBILE && <span className="plus-grab" aria-hidden="true" />}
        <DialogTitle className="attach-title">Attach files</DialogTitle>
        {IS_MOBILE ? (
          <>
            <div className="attach-sources">
              <button
                type="button"
                className="attach-btn"
                onClick={() => photosRef.current?.click()}
              >
                Photos
              </button>
              <button
                type="button"
                className="attach-btn"
                onClick={() => cameraRef.current?.click()}
              >
                Camera
              </button>
              <button
                type="button"
                className="attach-btn"
                onClick={() => filesRef.current?.click()}
              >
                Files
              </button>
            </div>
            <div className="attach-help">
              Images, PDFs, text and code, up to {LIMIT_MB} MB each.
            </div>
            <input
              ref={photosRef}
              type="file"
              accept="image/*"
              multiple
              hidden
              aria-label="Photos"
              onChange={handleBrowse}
            />
            <input
              ref={cameraRef}
              type="file"
              accept="image/*"
              capture="environment"
              hidden
              aria-label="Camera"
              onChange={handleBrowse}
            />
          </>
        ) : (
          <>
            <button type="button" className="attach-close" aria-label="Close" onClick={onClose}>
              <XIcon size={13} />
            </button>
            <div
              ref={dropzoneRef}
              data-testid="attach-file-dropzone"
              className="attach-drop"
              data-drop-target={dropTarget || undefined}
              onDragOver={handleDragOver}
              onDragLeave={handleDragLeave}
              onDrop={handleDrop}
            >
              <span className="attach-drop-big">
                Drop files here or{' '}
                <button
                  type="button"
                  className="attach-link"
                  onClick={() => filesRef.current?.click()}
                >
                  choose from your computer
                </button>
              </span>
              <span className="attach-drop-sm">
                Images, PDFs, text and code, up to {LIMIT_MB} MB each
              </span>
            </div>
          </>
        )}
        <input
          ref={filesRef}
          type="file"
          multiple
          hidden
          aria-label="Choose files"
          onChange={handleBrowse}
        />
        {chosen}
      </DialogPopup>
    </Dialog>
  )
}
