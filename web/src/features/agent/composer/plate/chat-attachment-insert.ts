import { NodeApi, PathApi, type Path, type Point, type TRange, type Value } from 'platejs'
import type { PlateEditor } from 'platejs/react'
import { CodeBlockPlugin } from '@platejs/code-block/react'
import { chatMarkdownToValue } from '@/features/agent/composer/plate/chat-composer-serialization'

/** An empty paragraph's path, so an insert goes at that index (the empty line
 *  trails the new content) instead of splitting it into two empty halves;
 *  null for real text, where an attachment lands inline at the caret. */
function emptyParagraphPathAt(
  editor: PlateEditor,
  at: Point | Path | TRange | undefined,
): Path | null {
  if (!at) return null
  const entry = editor.api.above({ at, match: { type: 'p' } })
  if (!entry) return null
  const [node, path] = entry
  return NodeApi.string(node) === '' ? path : null
}

/** Leaves the caret on one empty line after the attachment, so typing can go
 *  on; an already-empty last paragraph is reused, never stacked behind each. */
function ensureTrailingEditableLine(editor: PlateEditor): void {
  const last = editor.children.at(-1)
  const alreadyEmpty = !!last && last.type === 'p' && NodeApi.string(last) === ''
  if (!alreadyEmpty) {
    editor.tf.insertNodes([{ type: 'p', children: [{ text: '' }] }], {
      at: [editor.children.length],
    })
  }
  const end = editor.api.end([])
  if (end) editor.tf.select(end)
}

/** Inside a fence the insert goes after the whole code block: at the raw point
 *  it would split the fence and drop the new node. Only code blocks: walking
 *  to the highest block would split a table whose cell holds the caret. */
function attachmentInsertionPath(editor: PlateEditor): Path | Point | TRange | undefined {
  const at = editor.selection ?? editor.api.end([])
  const codeBlock = editor.api.above({ at, match: { type: CodeBlockPlugin.key } })
  return codeBlock ? PathApi.next(codeBlock[1]) : (emptyParagraphPathAt(editor, at) ?? at)
}

/** Inserts markdown through the composer's own codec, as a paste of it would. */
export function insertAttachmentMarkdownInto(editor: PlateEditor, markdown: string): void {
  const nodes = chatMarkdownToValue(markdown)
  editor.tf.insertNodes(nodes, { at: attachmentInsertionPath(editor) })
  ensureTrailingEditableLine(editor)
}

/** A preview at a local object URL before the upload starts. A raw node, not
 *  markdown, so a filename holding `]` or `)` can never misparse. */
export function insertPendingImageInto(editor: PlateEditor, objectUrl: string, alt: string): void {
  const node = {
    type: 'img',
    url: objectUrl,
    caption: [{ text: alt }],
    children: [{ text: '' }],
  } as unknown as Value[number]
  editor.tf.insertNodes([node], { at: attachmentInsertionPath(editor) })
  ensureTrailingEditableLine(editor)
}

/** Settles a pending image, found by its object URL since the person may have typed on: the
 *  real ref replaces it in place, `null` (a failed upload) removes it. Revokes the URL either way.
 */
export function settlePendingImageInto(
  editor: PlateEditor,
  objectUrl: string,
  finalMarkdown: string | null,
): void {
  const [entry] = Array.from(
    editor.api.nodes({
      at: [],
      match: (n) =>
        (n as { type?: string }).type === 'img' && (n as { url?: string }).url === objectUrl,
    }),
  )
  if (entry) {
    const [, path] = entry
    if (finalMarkdown === null) {
      editor.tf.removeNodes({ at: path })
    } else {
      const nodes = chatMarkdownToValue(finalMarkdown)
      const [finalNode] = nodes as unknown as { url?: string }[]
      if (finalNode?.url) {
        editor.tf.setNodes({ url: finalNode.url }, { at: path })
      } else {
        // The server sniffed a non-image: a void img pointing at it would block
        // Send, so the placeholder gives way to whatever it resolved to.
        editor.tf.withoutNormalizing(() => {
          editor.tf.removeNodes({ at: path })
          editor.tf.insertNodes(nodes, { at: path })
        })
      }
    }
  }
  URL.revokeObjectURL(objectUrl)
}
