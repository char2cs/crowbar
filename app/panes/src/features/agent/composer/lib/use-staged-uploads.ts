import { useCallback, useRef, useState } from 'react'
import { nanoid } from 'nanoid'
import type { UploadChatAttachmentInput } from '@/features/agent/api/upload-chat-attachment'
import { uploadAttachmentMarkdown } from '@/features/agent/composer/lib/attachment-upload'
import { LIMIT_MB, type Staged } from '@/features/agent/composer/lib/attach-staged'
import { UNREACHED, userWords } from '@/lib/errors/report'

const baseName = (path: string) => path.split(/[\\/]/).pop() ?? path

/** The files chosen so far, each uploading under its own abortable request. */
export function useStagedUploads(wsId: string, chatId: string) {
  const [items, setItems] = useState<Staged[]>([])
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
      const uploadId = nanoid()
      const send = () => {
        const controller = new AbortController()
        uploads.current.set(id, controller)
        settle(id, { status: 'uploading', loaded: undefined, problem: undefined, retry: undefined })
        uploadAttachmentMarkdown(wsId, chatId, input, {
          id: uploadId,
          signal: controller.signal,
          onProgress: (loaded) => settle(id, { loaded }),
        }).then(
          (markdown) => settle(id, { status: 'done', markdown }),
          (err: unknown) => {
            if (controller.signal.aborted) return
            settle(id, {
              status: 'bad',
              problem: userWords(err, UNREACHED) ?? UNREACHED,
              retry: send,
            })
          },
        )
      }
      send()
    },
    [wsId, chatId, settle],
  )

  const remove = useCallback((id: number) => {
    uploads.current.get(id)?.abort()
    uploads.current.delete(id)
    setItems((current) => current.filter((item) => item.id !== id))
  }, [])

  const abortAll = useCallback(() => {
    for (const controller of uploads.current.values()) controller.abort()
    uploads.current.clear()
  }, [])

  return { items, stage, remove, abortAll }
}
