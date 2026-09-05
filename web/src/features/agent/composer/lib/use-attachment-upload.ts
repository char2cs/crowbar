import { useCallback } from 'react'
import type { UploadChatAttachmentInput } from '@/features/agent/api/upload-chat-attachment'
import { uploadAttachmentMarkdown } from '@/features/agent/composer/lib/attachment-upload'
import { toast } from '@/features/window/stores/toast-store'

export interface UseAttachmentUploadResult {
  uploadAndInsert: (input: UploadChatAttachmentInput) => Promise<void>
}

/**
 * The upload half of every attach entry point — drop, browse, the Attach
 * File modal — collapsed into one hook instead of three near-identical
 * try/catch/toast blocks (`agent-composer.tsx`, `attach-file-modal.tsx`, and
 * now `agent-empty-document.tsx`). Resolves CSV inline where it fits
 * (`uploadAttachmentMarkdown`), otherwise uploads and hands the resulting
 * markdown to `onInsert`; a failure is caught and toasted here rather than
 * left for a caller to notice a silently-unresolved promise.
 *
 * Callers that need extra bookkeeping around the call (attach-file-modal.tsx's
 * `uploading` state, an `onClose`) wrap the returned `uploadAndInsert`
 * themselves — `onInsert` only fires on success, so wrapping this in a
 * `finally` for local state is safe without duplicating the try/catch.
 */
export function useAttachmentUpload(
  wsId: string,
  chatId: string,
  onInsert: (markdown: string) => void,
): UseAttachmentUploadResult {
  const uploadAndInsert = useCallback(
    async (input: UploadChatAttachmentInput) => {
      try {
        const markdown = await uploadAttachmentMarkdown(wsId, chatId, input)
        onInsert(markdown)
      } catch (err) {
        toast.error(
          'Could not attach that file',
          err instanceof Error ? err.message : 'Crowbar could not reach the daemon — try again.',
        )
      }
    },
    [wsId, chatId, onInsert],
  )

  return { uploadAndInsert }
}
