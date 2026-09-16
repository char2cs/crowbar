import { RangeApi, type TCodeBlockElement, type TElement } from 'platejs'
import type { PlateEditor } from 'platejs/react'
import { createPlatePlugin } from 'platejs/react'
import { parseAttachmentLang } from '@/features/agent/composer/plate/attachments/attachment-lang'

/** The one node type the box can always be typed into. */
const LINE = 'p'
const FENCE = 'code_block'
const IMAGE = 'img'

const emptyLine = (): TElement => ({ type: LINE, children: [{ text: '' }] })

/**
 * The index of the TOP-LEVEL block the collapsed caret sits at the very start
 * of, or null. Restricted to the top level on purpose: a caret at the start of
 * a table cell or a nested list block returns null, leaving ordinary
 * merge-into-the-previous-block behaviour untouched there.
 */
function topLevelBlockStart(editor: PlateEditor): number | null {
  const { selection } = editor
  if (!selection || !RangeApi.isCollapsed(selection)) return null
  const entry = editor.api.above({ match: (n) => editor.api.isBlock(n), mode: 'lowest' })
  if (!entry || entry[1].length !== 1) return null
  return editor.api.isStart(selection.anchor, entry[1]) ? entry[1][0] : null
}

const fenceLang = (node: TElement | undefined): string | undefined =>
  node?.type === FENCE ? ((node as TCodeBlockElement).lang ?? undefined) : undefined

/** A block with no editable surface to put a caret in: an image (a Slate
 *  void), or an attachment fence, whose raw code body renders inside a
 *  `hidden` div under its preview (chat-code-block-node.tsx) — a caret landing
 *  there is invisible and every character typed edits the attachment's own
 *  payload. */
function isAttachmentBlock(editor: PlateEditor, node: TElement | undefined): boolean {
  if (!node) return false
  return editor.api.isVoid(node) || !!parseAttachmentLang(fenceLang(node))
}

/** The first top-level index of the attachment occupying `index` — normally
 *  `index` itself, but an excalidraw attachment is a fence plus the persisted
 *  PNG right after it: one attachment, two nodes (see
 *  `removeExcalidrawAttachment`, chat-code-block-node.tsx, which deletes the
 *  pair together for the same reason). */
function attachmentStart(editor: PlateEditor, index: number): number {
  const isPngSibling =
    editor.children[index]?.type === IMAGE &&
    parseAttachmentLang(fenceLang(editor.children[index - 1]))?.kind === 'excalidraw'
  return isPngSibling ? index - 1 : index
}

/**
 * THE BOX'S STRUCTURAL FLOOR — the composer's own editor only.
 *
 * Both rules enforce one invariant: there is always a line you can type on,
 * and it is the last thing in the box.
 *
 * `normalizeNode` is `ensureTrailingEditableLine` (chat-markdown-editor.tsx)
 * made permanent — the same trailing line, re-checked after every operation
 * instead of once at insert time. Attaching an image gives `[img, p]`;
 * backspacing that empty line gave `[img]`, a void block alone with the caret
 * stranded inside it; deleting the attachment then gave `children: []` and
 * `selection: null`, which is not a valid Slate document — nothing renders,
 * the pill collapses to an empty oval, and no keystroke brings it back
 * (reported live, from exactly that order; the reverse order left a paragraph
 * and was always fine). `editor.tf.normalize({ force: true })` does NOT repair
 * it: Slate's core normalization covers an ELEMENT with no children, not the
 * editor root, and only `deleteFragment` restores a block by itself —
 * `removeNodes`, what the attachment's trash button calls, does not.
 *
 * `deleteBackward` gives that same backspace something to mean. With the rule
 * above alone it deletes the trailing line only for normalization to put it
 * straight back — a no-op that also drops the caret into the attachment it
 * was next to, where it is invisible and either eats every keystroke (an
 * image is void) or silently rewrites the attachment's payload (a fence's
 * code body is `hidden`, not absent). Backspace against an attachment removes
 * THE ATTACHMENT, which is what the person was reaching for.
 *
 * Registered on the composer's own editor (chat-markdown-editor.tsx), NOT on
 * the shared `chatComposerPlugins` — the transcript's streaming bubble renders
 * through that same set (markdown-message.tsx), and an answer is read, never
 * typed into.
 */
export const chatComposerStructurePlugin = createPlatePlugin({
  key: 'agent-chat-structure',
  // The same floor applied to the value the box OPENS with: Plate does not
  // normalize a `value:` it is handed, so a seeded draft ending in an
  // attachment (a queued prompt being edited) would otherwise open with
  // nowhere to type until its first edit.
  transformInitialValue: ({ value }) =>
    value.at(-1)?.type === LINE ? value : [...value, emptyLine()],
}).overrideEditor(({ editor, tf: { deleteBackward, normalizeNode } }) => ({
  transforms: {
    deleteBackward(unit) {
      const index = topLevelBlockStart(editor)
      if (index !== null && index > 0 && isAttachmentBlock(editor, editor.children[index - 1])) {
        const first = attachmentStart(editor, index - 1)
        editor.tf.withoutNormalizing(() => {
          for (let at = index - 1; at >= first; at--) editor.tf.removeNodes({ at: [at] })
        })
        return
      }
      deleteBackward(unit)
    },
    normalizeNode(entry) {
      if (entry[1].length === 0) {
        const last = editor.children.at(-1)
        if (!last || last.type !== LINE) {
          editor.tf.insertNodes(emptyLine(), { at: [editor.children.length] })
          // Removing the last node leaves `selection: null` behind (Slate has
          // nothing left to point at), and a null selection swallows the next
          // keystroke — so the box would come back rendering correctly and
          // still refuse to be typed in. Only for the emptied case: an
          // ordinary trailing line is appended without touching wherever the
          // caret already is.
          if (!last && !editor.selection) {
            const start = editor.api.start([])
            if (start) editor.tf.select(start)
          }
          return
        }
      }
      normalizeNode(entry)
    },
  },
}))
