import { createElement, useImperativeHandle, useRef } from 'react'
import type {
  ChatMarkdownEditorProps,
  ComposerShape,
} from '@/features/agent/composer/plate/chat-markdown-editor'

const shapeOf = (text: string): ComposerShape => ({
  empty: text.trim() === '',
  slashQuery: text.startsWith('/') ? (text.slice(1).split('\n')[0] ?? '') : null,
})

/**
 * The composer editor's contract over a textarea, for suites about the queue and
 * the pane rather than the editor: text in through the handle or typing, the
 * shape out on every change, keys through with the box's own text.
 */
export function TextareaComposerEditor({
  ariaLabel,
  placeholder,
  expanded,
  controls,
  onShapeChange,
  onKeyDown,
  ref,
}: ChatMarkdownEditorProps) {
  const box = useRef<HTMLTextAreaElement>(null)
  const load = (text: string) => {
    if (box.current) box.current.value = text
    onShapeChange(shapeOf(text))
  }
  const append = (markdown: string) => load(`${box.current?.value ?? ''}${markdown}`)
  useImperativeHandle(ref, () => ({
    insertAttachmentMarkdown: append,
    insertPendingImage: (objectUrl: string, alt: string) => append(`![${alt}](${objectUrl})`),
    settlePendingImage: () => {},
    getMarkdown: () => box.current?.value ?? '',
    setMarkdown: load,
    clear: () => load(''),
  }))
  return createElement('textarea', {
    ref: box,
    'aria-label': ariaLabel,
    'aria-expanded': expanded,
    'aria-controls': controls,
    placeholder,
    onChange: (event: { target: { value: string } }) => onShapeChange(shapeOf(event.target.value)),
    onKeyDown: (event: { currentTarget: { value: string } }) =>
      onKeyDown(
        event as never,
        () => event.currentTarget.value,
        { atStart: true, atEnd: true },
        shapeOf(event.currentTarget.value),
      ),
  })
}
