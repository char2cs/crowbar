import { NodeApi, PathApi, type Path, type Point, type TRange, type Value } from 'platejs'
import type { PlateEditor } from 'platejs/react'
import { CodeBlockPlugin } from '@platejs/code-block/react'
import { chatMarkdownToValue } from '@/features/agent/composer/plate/chat-composer-serialization'

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

function attachmentInsertionPath(editor: PlateEditor): Path | Point | TRange | undefined {
  const at = editor.selection ?? editor.api.end([])
  const codeBlock = editor.api.above({ at, match: { type: CodeBlockPlugin.key } })
  return codeBlock ? PathApi.next(codeBlock[1]) : (emptyParagraphPathAt(editor, at) ?? at)
}

export function insertAttachmentMarkdownInto(editor: PlateEditor, markdown: string): void {
  const nodes = chatMarkdownToValue(markdown)
  editor.tf.insertNodes(nodes, { at: attachmentInsertionPath(editor) })
  ensureTrailingEditableLine(editor)
}

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
        editor.tf.withoutNormalizing(() => {
          editor.tf.removeNodes({ at: path })
          editor.tf.insertNodes(nodes, { at: path })
        })
      }
    }
  }
  URL.revokeObjectURL(objectUrl)
}
