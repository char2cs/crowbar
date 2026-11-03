import { useEffect, useMemo, useRef } from 'react'
import type { Value } from 'platejs'
import { Plate, PlateContent, usePlateEditor } from 'platejs/react'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'
import { chatMarkdownToValue } from '@/features/agent/composer/plate/chat-composer-serialization'
import {
  FRESH_CURSOR,
  openDiagramFence,
  parseStreamingMarkdown,
  type StreamCursor,
} from '@/features/agent/transcript/plate/streaming-markdown-parse'
import { CHAT_PROSE_STYLE } from '@/features/agent/transcript/plate/chat-prose-style'
import { StreamingFenceContext } from '@/features/agent/transcript/plate/streaming-fence'
import { applyStreamedValue } from '@/features/agent/transcript/plate/streaming-value-patch'
import { markEnd, markStart } from '@/lib/perf/instrumentation'
import { cn } from '@/lib/utils'

interface MarkdownMessageProps {
  /** The message's markdown. */
  children: string
  /** The reply is over; a diagram fence it left open then renders as code. */
  settled?: boolean
  className?: string
}

/**
 * A streaming message, on the composer's own Plate plugin set (one engine, one
 * appearance). Interactive only so `applyStreamedValue` can patch the document
 * in place as tokens arrive; a message settled at mount renders static.
 */
export function MarkdownMessage({ children, settled = false, className }: MarkdownMessageProps) {
  const initialValueRef = useRef<Value | undefined>(undefined)
  const appliedChildrenRef = useRef(children)
  const cursorRef = useRef<StreamCursor>(FRESH_CURSOR)
  initialValueRef.current ??= chatMarkdownToValue(children)

  // Created once for the bubble's life and patched as `children` grows:
  // rebuilding it per token rebuilt the whole DOM subtree.
  const editor = usePlateEditor(
    { plugins: chatComposerPlugins, value: initialValueRef.current },
    [],
  )

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
    const raf = requestAnimationFrame(() => markEnd('chat.stream.token'))
    return () => cancelAnimationFrame(raf)
  }, [children, editor])

  // The settled prefix is never inside a fence, so the scan starts at the cursor.
  const openFence = useMemo(() => {
    const cursor = cursorRef.current
    return openDiagramFence(children, children.startsWith(cursor.prefix) ? cursor.offset : 0)
  }, [children])
  const live = !settled
  const fence = useMemo(() => ({ live, openFence }), [live, openFence])

  return (
    <StreamingFenceContext.Provider value={fence}>
      <Plate editor={editor} readOnly>
        <PlateContent
          readOnly
          tabIndex={-1}
          style={CHAT_PROSE_STYLE}
          className={cn('agent-prose', className)}
        />
      </Plate>
    </StreamingFenceContext.Provider>
  )
}
