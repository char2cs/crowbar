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
  const write = (text: string, edited: boolean) => {
    if (box.current) box.current.value = text
    onShapeChange(shapeOf(text), edited)
  }
  const load = (text: string) => write(text, false)
  const append = (markdown: string) => write(`${box.current?.value ?? ''}${markdown}`, true)
  const typed = () => box.current?.value.trimEnd() ?? ''
  useImperativeHandle(ref, () => ({
    insertAttachmentMarkdown: append,
    insertPendingImage: (objectUrl: string, alt: string) => append(`![${alt}](${objectUrl})`),
    settlePendingImage: () => {},
    getMarkdown: () => box.current?.value ?? '',
    setMarkdown: load,
    appendMarkdown: (text: string) => (typed() ? write(`${typed()}\n\n${text}`, true) : load(text)),
    focus: () => box.current?.focus(),
    clear: () => load(''),
    restoreMarkdown: load,
  }))
  return createElement('textarea', {
    ref: box,
    'aria-label': ariaLabel,
    'aria-expanded': expanded,
    'aria-controls': controls,
    placeholder,
    onChange: (event: { target: { value: string } }) =>
      onShapeChange(shapeOf(event.target.value), true),
    onKeyDown: (event: { currentTarget: { value: string } }) =>
      onKeyDown(
        event as never,
        () => event.currentTarget.value,
        { atStart: true, atEnd: true },
        shapeOf(event.currentTarget.value),
      ),
  })
}
