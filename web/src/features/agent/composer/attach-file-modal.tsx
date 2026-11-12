import type { ChangeEvent, DragEvent, KeyboardEvent } from 'react'
import { useCallback, useRef, useState } from 'react'
import { XIcon } from '@phosphor-icons/react'
import { Dialog, DialogPopup, DialogTitle } from '@/components/ui/dialog'
import { AttachFileList } from '@/features/agent/composer/attach-file-list'
import { useStagedUploads } from '@/features/agent/composer/lib/use-staged-uploads'
import { useTauriFileDrop } from '@/features/file-system/lib/tauri-file-drop'
import { NativeAttachSheetView, useOnCancel } from '@/features/agent/composer/native-attach-sheet'
import { attachSheetFiles, LIMIT_MB, summarize } from '@/features/agent/composer/lib/attach-staged'
import { hasNativeChrome, type NativeChromeIntent } from '@/lib/native-chrome'
import { IS_MOBILE } from '@/utils/platform'

interface AttachFileModalProps {
  wsId: string
  chatId: string
  open: boolean
  onClose: () => void
  onInsertMarkdown: (markdown: string) => void
}

/** Files are uploaded as they are chosen and listed with their state; "Attach" puts the finished
 *  ones into the message. A modal, not inline: the pill has no room to grow a picker. */
export function AttachFileModal({
  wsId,
  chatId,
  open,
  onClose,
  onInsertMarkdown,
}: AttachFileModalProps) {
  const [dropTarget, setDropTarget] = useState(false)
  // The native sheet steps aside while the system picker is up and returns when it answers.
  const [picking, setPicking] = useState(false)
  const dropzoneRef = useRef<HTMLDivElement>(null)
  const filesRef = useRef<HTMLInputElement>(null)
  const photosRef = useRef<HTMLInputElement>(null)
  const cameraRef = useRef<HTMLInputElement>(null)

  const { items, stage, remove, abortAll } = useStagedUploads(wsId, chatId)

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
    setPicking(false)
    for (const file of Array.from(e.target.files ?? [])) stage({ file })
    e.target.value = ''
  }

  const { ready, attachLabel, hint } = summarize(items)

  const attach = () => {
    if (!ready) return
    for (const item of items)
      if (item.status === 'done' && item.markdown) onInsertMarkdown(item.markdown)
    onClose()
  }

  const handleKeyDown = (e: KeyboardEvent) => {
    if (e.key !== 'Enter' || e.target instanceof HTMLButtonElement) return
    e.preventDefault()
    attach()
  }

  // Closing without attaching abandons what is still on its way.
  const cancel = () => {
    abortAll()
    onClose()
  }

  const chosen = (
    <>
      <AttachFileList items={items} onRemove={remove} />
      <div className="attach-foot">
        {hint && <span className="attach-hint">{hint}</span>}
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

  const sources = [
    { id: 'photos', title: 'Photos', icon: 'Images', input: photosRef },
    { id: 'camera', title: 'Camera', icon: 'Camera', input: cameraRef },
    { id: 'files', title: 'Files', icon: 'Folder', input: filesRef },
  ]
  const onNativeIntent = (intent: NativeChromeIntent) => {
    if (intent.type === 'pickAttachSource') {
      setPicking(true)
      sources.find((source) => source.id === intent.id)?.input.current?.click()
    }
    if (intent.type === 'removeAttachFile') remove(Number(intent.id))
    if (intent.type === 'retryAttachFile') {
      items.find((item) => String(item.id) === intent.id)?.retry?.()
    }
    if (intent.type === 'attachFiles') attach()
    if (intent.type === 'closeAttachSheet') cancel()
  }

  // A dismissed system picker reports `cancel`, not `change`.
  const back = useCallback(() => setPicking(false), [])
  useOnCancel(photosRef, picking, back)
  useOnCancel(cameraRef, picking, back)
  useOnCancel(filesRef, picking, back)

  if (hasNativeChrome()) {
    return (
      <>
        <NativeAttachSheetView
          open={open && !picking}
          sheet={{
            title: 'Attach files',
            note: `Images, PDFs, text and code, up to ${LIMIT_MB} MB each.`,
            sources: sources.map(({ id, title, icon }) => ({ id, title, icon })),
            files: attachSheetFiles(items),
            hint,
            attachLabel,
            canAttach: ready,
          }}
          onIntent={onNativeIntent}
        />
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
        <input
          ref={filesRef}
          type="file"
          multiple
          hidden
          aria-label="Choose files"
          onChange={handleBrowse}
        />
      </>
    )
  }

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
