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
  onHeightChange: (height: number) => void
  ref?: Ref<ChatMarkdownEditorHandle>
}

const DOCK_STYLE = { minHeight: COMPOSER_LINE_HEIGHT } satisfies CSSProperties

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
      persistAs={chatId}
      document={document}
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
