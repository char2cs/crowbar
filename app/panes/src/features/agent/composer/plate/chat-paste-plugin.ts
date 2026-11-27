import { createPlatePlugin } from 'platejs/react'
import { CodeBlockPlugin } from '@platejs/code-block/react'
import { nanoid } from 'nanoid'
import { shouldWrapAsTextAttachment } from '@/features/agent/composer/lib/paste-threshold'
import { exceedsInlineSizeCap } from '@/features/agent/composer/lib/inline-attachment-cap'
import {
  imageMarkdown,
  textAttachmentMarkdown,
} from '@/features/agent/composer/lib/attachment-markdown'
import { uploadAttachmentMarkdown } from '@/features/agent/composer/lib/attachment-upload'
import { uploadChatAttachment } from '@/features/agent/api/upload-chat-attachment'
import {
  insertAttachmentMarkdownInto,
  insertPendingImageInto,
  settlePendingImageInto,
} from '@/features/agent/composer/plate/chat-attachment-insert'
import { report, UNREACHED } from '@/lib/errors/report'

interface ChatPastePluginOptions {
  wsId: string
  chatId: string
}

const ATTACHMENT_FENCE_LANG_PREFIXES = ['text-attachment:', 'excalidraw:']

function isRealCodeBlock(node: { type?: string; lang?: unknown }): boolean {
  if (node.type !== CodeBlockPlugin.key) return false
  const lang = typeof node.lang === 'string' ? node.lang : ''
  return !ATTACHMENT_FENCE_LANG_PREFIXES.some((prefix) => lang.startsWith(prefix))
}

export function createChatPastePlugin({ wsId, chatId }: ChatPastePluginOptions) {
  let shiftHeld = false

  return createPlatePlugin({
    key: 'agent-chat-paste',
    handlers: {
      onKeyDown: ({ event }) => {
        if (event.key === 'Shift') shiftHeld = true
      },
      onKeyUp: ({ event }) => {
        if (event.key === 'Shift') shiftHeld = false
      },
      onPaste: ({ editor, event }) => {
        if (shiftHeld) return

        const inCodeBlock = editor.api.above({ match: isRealCodeBlock })
        if (inCodeBlock) return

        const clipboard = event.clipboardData
        const imageItem = Array.from(clipboard?.items ?? []).find((item) =>
          item.type.startsWith('image/'),
        )
        if (imageItem) {
          event.preventDefault()
          const file = imageItem.getAsFile()
          if (!file) return true
          // react-doctor-disable-next-line no-create-object-url-without-revoke -- revoked in chat-attachment-insert.ts's settlePendingImageInto, called on both the .then and .catch below with this same objectUrl; the rule can't trace it across that async boundary.
          const objectUrl = URL.createObjectURL(file)
          insertPendingImageInto(editor, objectUrl, file.name)
          void uploadChatAttachment(wsId, chatId, { file })
            .then((result) => {
              settlePendingImageInto(editor, objectUrl, imageMarkdown(result.filename, result.ref))
            })
            .catch((err) => {
              settlePendingImageInto(editor, objectUrl, null)
              report(err, { action: 'Could not attach that image', fallback: UNREACHED })
            })
          return true
        }

        const otherFiles = Array.from(clipboard?.files ?? [])
        if (otherFiles.length > 0) {
          event.preventDefault()
          for (const file of otherFiles) {
            void uploadAttachmentMarkdown(wsId, chatId, { file })
              .then((markdown) => {
                insertAttachmentMarkdownInto(editor, markdown)
              })
              .catch((err) => {
                report(err, { action: 'Could not attach that file', fallback: UNREACHED })
              })
          }
          return true
        }

        const text = clipboard?.getData('text/plain') ?? ''
        if (!shouldWrapAsTextAttachment(text)) return

        event.preventDefault()

        if (exceedsInlineSizeCap(text)) {
          const file = new File([text], 'pasted.txt', { type: 'text/plain' })
          void uploadAttachmentMarkdown(wsId, chatId, { file })
            .then((markdown) => {
              insertAttachmentMarkdownInto(editor, markdown)
            })
            .catch((err) => {
              report(err, { action: 'Could not attach that text', fallback: UNREACHED })
            })
          return true
        }

        insertAttachmentMarkdownInto(editor, textAttachmentMarkdown(nanoid(), text))
        return true
      },
    },
  })
}
