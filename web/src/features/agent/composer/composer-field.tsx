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

// The composer's one-line field that grows. Rich, not a textarea: the agent reads it and the
// transcript renders it as markdown, so the box shows `**bold**` the way the rest of the chat does.
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
