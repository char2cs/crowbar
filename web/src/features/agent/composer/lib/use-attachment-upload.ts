import { useCallback } from 'react'
import type { UploadChatAttachmentInput } from '@/features/agent/api/upload-chat-attachment'
import { uploadAttachmentMarkdown } from '@/features/agent/composer/lib/attachment-upload'
import { report, UNREACHED } from '@/lib/errors/report'

export interface UseAttachmentUploadResult {
  uploadAndInsert: (input: UploadChatAttachmentInput) => Promise<void>
}

// Every attach entry point's upload: markdown goes to `onInsert` on success, a failure is reported
// here and never rejects. With both pending-image callbacks, an image File (a host path has no bytes
// to preview) shows a local preview at once and settles it instead of calling `onInsert`.
export function useAttachmentUpload(
  wsId: string,
  chatId: string,
  onInsert: (markdown: string) => void,
  onInsertPendingImage?: (objectUrl: string, alt: string) => void,
  onSettlePendingImage?: (objectUrl: string, finalMarkdown: string | null) => void,
): UseAttachmentUploadResult {
  const uploadAndInsert = useCallback(
    async (input: UploadChatAttachmentInput) => {
      const insertPending = onInsertPendingImage
      const settlePending = onSettlePendingImage
      const imageFile =
        insertPending && settlePending && 'file' in input && input.file.type.startsWith('image/')
          ? input.file
          : null

      if (imageFile && insertPending && settlePending) {
        // react-doctor-disable-next-line no-create-object-url-without-revoke -- revoked in chat-attachment-insert.ts's settlePendingImageInto, which every branch below (success and catch) hands this same objectUrl to; the rule can't trace it across the settlePending callback boundary.
        const objectUrl = URL.createObjectURL(imageFile)
        insertPending(objectUrl, imageFile.name)
        try {
          const markdown = await uploadAttachmentMarkdown(wsId, chatId, input)
          settlePending(objectUrl, markdown)
        } catch (err) {
          settlePending(objectUrl, null)
          report(err, { action: 'Could not attach that file', fallback: UNREACHED })
        }
        return
      }

      try {
        const markdown = await uploadAttachmentMarkdown(wsId, chatId, input)
        onInsert(markdown)
      } catch (err) {
        report(err, { action: 'Could not attach that file', fallback: UNREACHED })
      }
    },
    [wsId, chatId, onInsert, onInsertPendingImage, onSettlePendingImage],
  )

  return { uploadAndInsert }
}
