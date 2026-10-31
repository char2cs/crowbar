import type { CSSProperties, KeyboardEvent, Ref } from 'react'
import {
  ChatMarkdownEditor,
  type CaretEdges,
  type ChatMarkdownEditorHandle,
  type ComposerShape,
} from '@/features/agent/composer/plate/chat-markdown-editor'
import { COMPOSER_LINE_HEIGHT } from '@/features/agent/composer/lib/handle-geometry'

interface ComposerFieldProps {
  wsId: string
  chatId: string
  placement: 'document' | 'dock'
  placeholder: string
  expanded: boolean
  controls?: string
  onShapeChange: (shape: ComposerShape, edited: boolean) => void
  onKeyDown: (
    event: KeyboardEvent<HTMLDivElement>,
    readMarkdown: () => string,
    caret: CaretEdges,
    shape: ComposerShape,
  ) => void
  /** Reported on every change, so the handle can ride the last line. */
  onHeightChange: (height: number) => void
  /** Forwarded straight through to `ChatMarkdownEditor` — see
   *  `ChatMarkdownEditorHandle` for what a caller outside the field can do
   *  with it. */
  ref?: Ref<ChatMarkdownEditorHandle>
}

// A style prop, not a class: Slate's Editable writes `min-height: 20px` INLINE,
// and spreads the caller's style after its own defaults.
const DOCK_STYLE = { minHeight: COMPOSER_LINE_HEIGHT } satisfies CSSProperties

/**
 * The composer's text field: one line of markdown that grows.
 *
 * Rich rather than a textarea, because what is typed here IS markdown — the
 * agent reads it as markdown, and the transcript renders the answer as markdown.
 * A box that showed `**bold**` as four characters and five letters was the one
 * place in the conversation where the same text meant two different things.
 */
export function ComposerField({
  wsId,
  chatId,
  placement,
  placeholder,
  expanded,
  controls,
  onShapeChange,
  onKeyDown,
  onHeightChange,
  ref,
}: ComposerFieldProps) {
  const document = placement === 'document'
  return (
    <ChatMarkdownEditor
      ref={ref}
      wsId={wsId}
      chatId={chatId}
      placeholder={placeholder}
      ariaLabel={document ? 'Describe the change' : 'Message the agent'}
      expanded={expanded}
      controls={controls}
      autoFocus
      onShapeChange={onShapeChange}
      onKeyDown={onKeyDown}
      onHeightChange={onHeightChange}
      className={document ? 'blk' : undefined}
      style={document ? undefined : DOCK_STYLE}
    />
  )
}
