import type { CSSProperties, KeyboardEvent, Ref } from 'react'
import {
  ChatMarkdownEditor,
  type CaretEdges,
  type ChatMarkdownEditorHandle,
} from '@/features/agent/composer/plate/chat-markdown-editor'
import { COMPOSER_LINE_HEIGHT } from '@/features/agent/composer/lib/handle-geometry'

interface ComposerFieldProps {
  wsId: string
  chatId: string
  /** The draft to OPEN with. The box owns its text after that — see the editor. */
  initialValue: string
  placeholder: string
  expanded: boolean
  controls?: string
  onChange: (value: string) => void
  onKeyDown: (
    event: KeyboardEvent<HTMLDivElement>,
    readMarkdown: () => string,
    caret: CaretEdges,
  ) => void
  /** Reported on every change, so the handle can ride the last line. */
  onHeightChange: (height: number) => void
  /** Forwarded straight through to `ChatMarkdownEditor` — see
   *  `ChatMarkdownEditorHandle` for what a caller outside the field can do
   *  with it. */
  ref?: Ref<ChatMarkdownEditorHandle>
}

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
  initialValue,
  placeholder,
  expanded,
  controls,
  onChange,
  onKeyDown,
  onHeightChange,
  ref,
}: ComposerFieldProps) {
  return (
    <ChatMarkdownEditor
      ref={ref}
      wsId={wsId}
      chatId={chatId}
      initialValue={initialValue}
      placeholder={placeholder}
      ariaLabel="Message the agent"
      expanded={expanded}
      controls={controls}
      autoFocus
      onChange={onChange}
      onKeyDown={onKeyDown}
      onHeightChange={onHeightChange}
      // A style prop, not a class: Slate's Editable writes `min-height: 20px`
      // INLINE, which beats any class. Slate spreads the caller's style after
      // its own defaults, so this is the one place the value can be set.
      style={{ minHeight: COMPOSER_LINE_HEIGHT } satisfies CSSProperties}
    />
  )
}
