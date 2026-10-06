import { useEffect, useRef } from 'react'
import type { Value } from 'platejs'
import { Plate, PlateContent, usePlateEditor } from 'platejs/react'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'
import { chatMarkdownToValue } from '@/features/agent/composer/plate/chat-composer-serialization'
import {
  FRESH_CURSOR,
  parseStreamingMarkdown,
  type StreamCursor,
} from '@/features/agent/transcript/plate/streaming-markdown-parse'
import { applyStreamedValue } from '@/features/agent/transcript/plate/streaming-value-patch'
import { markEnd, markStart } from '@/lib/perf/instrumentation'
import { cn } from '@/lib/utils'

interface MarkdownMessageProps {
  /** The message's markdown. */
  children: string
  className?: string
}

/**
 * A message, rendered.
 *
 * PLATE, over the same plugin set the composer writes with — not
 * `react-markdown`. One engine is the whole point: a table, a callout or an
 * inline code span has exactly one appearance in Crowbar, and a fix to how one
 * renders cannot leave the other behind. It also means what an agent sends back
 * and what you type next are parsed by the same rules.
 *
 * A real, interactive editor only because this is the actively-streaming
 * bubble: `applyStreamedValue` patches an existing Plate document in place as
 * tokens arrive, which only works on a real `usePlateEditor` instance. Every
 * settled message renders through `MarkdownMessageStatic` instead, over the
 * same derived plugin set (`chatComposerPluginsStatic`) — so there is no
 * second definition of what a heading looks like anywhere in this pair.
 */
export function MarkdownMessage({ children, className }: MarkdownMessageProps) {
  // Parsed once, lazily, for the editor's initial value — a lazy ref rather
  // than useMemo so it truly runs only on the first render, never once per
  // rerender to be thrown away.
  const initialValueRef = useRef<Value | undefined>(undefined)
  const appliedChildrenRef = useRef(children)
  const cursorRef = useRef<StreamCursor>(FRESH_CURSOR)
  initialValueRef.current ??= chatMarkdownToValue(children)

  // Deps stay empty: this editor is created ONCE for the life of the bubble
  // and PATCHED as `children` grows, below. Keying it to `value` (the old
  // design) rebuilt the whole editor — and therefore the whole DOM subtree —
  // on every streamed token, which is the per-token cost the 2026-08-24
  // performance plan measured.
  const editor = usePlateEditor(
    { plugins: chatComposerPlugins, value: initialValueRef.current },
    [],
  )

  // Re-parsed and applied only when the text actually changes. Only the open
  // tail is parsed (streaming-markdown-parse.ts); the settled blocks come from
  // the editor itself. A streaming message changes on every token and this is
  // its hot path: `applyStreamedValue`
  // touches only the blocks that differ from what the editor already holds
  // (see streaming-value-patch.ts) instead of rebuilding the document whole.
  // markStart/markEnd fire exactly once per distinct text value — including
  // the first, already reflected in `editor`'s initial value — never on a
  // same-text re-render.
  useEffect(() => {
    markStart('chat.stream.token')
    if (children !== appliedChildrenRef.current) {
      const step = parseStreamingMarkdown(
        children,
        cursorRef.current,
        editor.children as Value,
        chatMarkdownToValue,
      )
      cursorRef.current = step.cursor
      applyStreamedValue(editor, step.value)
      appliedChildrenRef.current = children
    }
    // Paint-inclusive: rAF defers markEnd past the commit this value produced,
    // the same pattern workspace.switch uses for its cold-path span.
    const raf = requestAnimationFrame(() => markEnd('chat.stream.token'))
    return () => cancelAnimationFrame(raf)
  }, [children, editor])

  return (
    <Plate editor={editor} readOnly>
      <PlateContent readOnly tabIndex={-1} className={cn('agent-prose', className)} />
    </Plate>
  )
}
