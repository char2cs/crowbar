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
import { insertAttachmentMarkdownInto } from '@/features/agent/composer/plate/chat-markdown-editor'
import { toast } from '@/features/window/stores/toast-store'

interface ChatPastePluginOptions {
  wsId: string
  chatId: string
}

/**
 * Paste interception, as a plugin's `handlers` — NOT the `PlateContent` DOM
 * prop. Same reasoning as `agent-chat-keys`'s onKeyDown (chat-markdown-
 * editor.tsx): a plugin handler runs before slate-react's own `onPaste`
 * (`isEventHandled` there checks `event.isDefaultPrevented()`), so this is
 * the last point a `preventDefault()` can still stop Slate's own insertion.
 *
 * Order:
 *  1. Shift held at paste time -> bypass everything, default paste happens.
 *     The browser `paste` event carries no modifier info of its own, so
 *     Shift is tracked separately via this SAME plugin's onKeyDown/onKeyUp
 *     — deliberately independent of `agent-chat-keys`, which owns Enter/
 *     Cmd+A and has nothing to do with paste.
 *  2. Caret inside a code block -> do nothing, let default paste happen.
 *  3. Clipboard has image data -> always intercepted, uploads + inserts an
 *     image node.
 *  4. Plain text over threshold, under the shared inline size cap -> wrapped
 *     as a `text-attachment` fence.
 *  4b. Plain text over the inline size cap (`inline-attachment-cap.ts`,
 *     shared with the Excalidraw save path) -> uploaded as a `.txt` file
 *     instead, same reasoning as `MAX_PROMPT_TEXT_BYTES`'s own note: an
 *     inline fence that big can push a whole draft past the 64KB ceiling
 *     with no way to fix it once typed.
 *  5. Otherwise -> default paste happens (short plain text).
 */
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

        const inCodeBlock = editor.api.above({ match: { type: CodeBlockPlugin.key } })
        if (inCodeBlock) return

        const clipboard = event.clipboardData
        const imageItem = Array.from(clipboard?.items ?? []).find((item) =>
          item.type.startsWith('image/'),
        )
        if (imageItem) {
          event.preventDefault()
          const file = imageItem.getAsFile()
          if (!file) return
          // The selection to insert at is read fresh, INSIDE
          // `insertAttachmentMarkdownInto`, once the upload resolves — not
          // captured here before the `await`. A Slate `Point`/`Path`
          // captured now is a position in the CURRENT document; anything the
          // person types while the upload is in flight can shift or
          // invalidate it before `insertNodes` ever runs. Reading
          // `editor.selection` at insertion time instead is what every other
          // async attach path in this composer already does (agent-
          // composer.tsx's `uploadAndInsert` -> the imperative
          // `insertAttachmentMarkdown` handle, for a drop or the Attach File
          // modal) — it inserts at wherever the caret actually is once the
          // network round trip completes, never at a stale location.
          void uploadChatAttachment(wsId, chatId, { file })
            .then((result) => {
              insertAttachmentMarkdownInto(editor, imageMarkdown(result.filename, result.ref))
            })
            .catch((err) => {
              // Unhandled otherwise: a paste of a large/rejected image would
              // silently do nothing, mid-typing, with no feedback at all.
              toast.error(
                'Could not attach that image',
                err instanceof Error
                  ? err.message
                  : 'Crowbar could not reach the daemon — try again.',
              )
            })
          return
        }

        const text = clipboard?.getData('text/plain') ?? ''
        if (!shouldWrapAsTextAttachment(text)) return

        event.preventDefault()

        if (exceedsInlineSizeCap(text)) {
          // Same "read selection fresh at insertion time" reasoning as the
          // image-paste branch above — an async upload separates the paste
          // from the insert by a network round trip, and the caret may have
          // moved by the time it resolves.
          const file = new File([text], 'pasted.txt', { type: 'text/plain' })
          void uploadAttachmentMarkdown(wsId, chatId, { file })
            .then((markdown) => {
              insertAttachmentMarkdownInto(editor, markdown)
            })
            .catch((err) => {
              toast.error(
                'Could not attach that text',
                err instanceof Error
                  ? err.message
                  : 'Crowbar could not reach the daemon — try again.',
              )
            })
          return
        }

        insertAttachmentMarkdownInto(editor, textAttachmentMarkdown(nanoid(), text))
      },
    },
  })
}
