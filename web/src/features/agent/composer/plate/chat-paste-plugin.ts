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
} from '@/features/agent/composer/plate/chat-markdown-editor'
import { toast } from '@/features/window/stores/toast-store'

interface ChatPastePluginOptions {
  wsId: string
  chatId: string
}

/** `lang` prefixes `attachment-markdown.ts` fences its two kinds with — the
 *  same strings the render-side plugin keys off of to tell a real fenced
 *  code block apart from a collapsed attachment pill (see that file's own
 *  note on why the id suffix is load-bearing). */
const ATTACHMENT_FENCE_LANG_PREFIXES = ['text-attachment:', 'excalidraw:']

/** Is `node` a `code_block` a person is actually editing as code — as
 *  opposed to one of THIS plugin's own attachment fences. Both are the same
 *  Plate node type, so the caret sitting inside either looks identical to a
 *  bare `{ type: CodeBlockPlugin.key }` match.
 *
 *  The distinction matters because a `text-attachment`/`excalidraw` fence is
 *  never meant to be typed into — `insertAttachmentMarkdownInto` (chat-
 *  markdown-editor.tsx) leaves the selection sitting inside the fence's own
 *  `code_line` right after inserting it (documented on that function), which
 *  is indistinguishable, to a blanket `{type: CodeBlockPlugin.key}` match,
 *  from a caret a person deliberately parked inside a REAL typed code block.
 *  A bare match here previously meant a second paste with no intervening
 *  keystroke — the caret still sitting exactly there — silently bypassed
 *  this plugin entirely and fell through to Slate's own default paste
 *  handling, which appended the new text into the SAME fence instead of
 *  starting its own (Wave 6, Bug 2: two pastes merged into one pill). */
function isRealCodeBlock(node: { type?: string; lang?: unknown }): boolean {
  if (node.type !== CodeBlockPlugin.key) return false
  const lang = typeof node.lang === 'string' ? node.lang : ''
  return !ATTACHMENT_FENCE_LANG_PREFIXES.some((prefix) => lang.startsWith(prefix))
}

/**
 * Paste interception, as a plugin's `handlers` — NOT the `PlateContent` DOM
 * prop. Every branch that intercepts a paste must both call
 * `event.preventDefault()` (stops the browser's own paste) AND `return true`
 * (stops Plate's `pipeHandler` from also running Slate's default insertion —
 * `preventDefault()` alone does not; reported live as the raw pasted text
 * landing a second time, right below the fence this plugin had just inserted).
 *
 * Order:
 *  1. Shift held at paste time -> bypass everything, default paste happens.
 *     The browser `paste` event carries no modifier info of its own, so
 *     Shift is tracked separately via this SAME plugin's onKeyDown/onKeyUp
 *     — deliberately independent of `agent-chat-keys`, which owns Enter/
 *     Cmd+A and has nothing to do with paste.
 *  2. Caret inside a REAL (not attachment-fence) code block -> do nothing,
 *     let default paste happen — see `isRealCodeBlock` above.
 *  3. Clipboard has image data -> always intercepted, uploads + inserts an
 *     image node.
 *  3b. Clipboard carries any OTHER file (a PDF copied in Finder, a CSV, a
 *     docx, ...) -> uploaded through the same `uploadAttachmentMarkdown` the
 *     drop and Attach File paths already share, so it gets the identical
 *     image/file/CSV-table resolution by content-type.
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
          // Optimistic: a local `URL.createObjectURL` preview goes in
          // immediately, right where the caret is AT PASTE TIME — reported
          // live as "photos attachments are not loaded instantly... let's
          // not wait for them." `settlePendingImageInto` below finds this
          // same node by its OWN object url once the upload resolves, not by
          // position, so it's unaffected by anything typed in the meantime —
          // unlike the position-based insert this replaced (which used to
          // defer reading the selection until the upload settled specifically
          // to dodge a stale position), there is no position left to go stale.
          // react-doctor-disable-next-line no-create-object-url-without-revoke -- revoked in chat-markdown-editor.tsx's settlePendingImageInto, called on both the .then and .catch below with this same objectUrl; the rule can't trace it across that async boundary.
          const objectUrl = URL.createObjectURL(file)
          insertPendingImageInto(editor, objectUrl, file.name)
          void uploadChatAttachment(wsId, chatId, { file })
            .then((result) => {
              settlePendingImageInto(editor, objectUrl, imageMarkdown(result.filename, result.ref))
            })
            .catch((err) => {
              settlePendingImageInto(editor, objectUrl, null)
              // Unhandled otherwise: a paste of a large/rejected image would
              // silently do nothing, mid-typing, with no feedback at all.
              toast.error(
                'Could not attach that image',
                err instanceof Error
                  ? err.message
                  : 'Crowbar could not reach the daemon — try again.',
              )
            })
          return true
        }

        // A file that isn't an image — a PDF, a CSV, a docx, anything copied
        // in the OS file manager rather than a browser. `clipboardData.files`
        // is where the browser actually exposes that File's bytes; it is
        // never populated for a plain text/image copy, so this only fires for
        // a genuine file paste. Without this branch such a paste had no
        // `image/*` item and no `text/plain` payload either, so Cmd/Ctrl+V
        // silently did nothing at all.
        const otherFiles = Array.from(clipboard?.files ?? [])
        if (otherFiles.length > 0) {
          event.preventDefault()
          for (const file of otherFiles) {
            void uploadAttachmentMarkdown(wsId, chatId, { file })
              .then((markdown) => {
                insertAttachmentMarkdownInto(editor, markdown)
              })
              .catch((err) => {
                toast.error(
                  'Could not attach that file',
                  err instanceof Error
                    ? err.message
                    : 'Crowbar could not reach the daemon — try again.',
                )
              })
          }
          return true
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
          return true
        }

        insertAttachmentMarkdownInto(editor, textAttachmentMarkdown(nanoid(), text))
        return true
      },
    },
  })
}
