import { useEffect, useMemo } from 'react'
import { Plate, PlateContent, usePlateEditor } from 'platejs/react'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'
import { chatMarkdownToValue } from '@/features/agent/composer/plate/chat-composer-serialization'
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
 * Read-only rather than static: Plate ships no static components in this app, so
 * a `PlateStatic` render would need a second set of node components — a second
 * definition of what a heading looks like, which is the divergence this exists
 * to prevent. A read-only editor reuses the ones already here.
 */
export function MarkdownMessage({ children, className }: MarkdownMessageProps) {
  // Re-parsed only when the text actually changes. A streaming message changes
  // on every token and this is its hot path — markStart lives INSIDE the
  // memo so it fires exactly when a new value is computed, never on a
  // same-text re-render, keeping every markStart paired with exactly one
  // markEnd below.
  const value = useMemo(() => {
    markStart('chat.stream.token')
    return chatMarkdownToValue(children)
  }, [children])
  const editor = usePlateEditor({ plugins: chatComposerPlugins, value }, [value])

  // Paint-inclusive: rAF defers markEnd past the commit this value produced,
  // the same pattern workspace.switch uses for its cold-path span.
  useEffect(() => {
    const raf = requestAnimationFrame(() => markEnd('chat.stream.token'))
    return () => cancelAnimationFrame(raf)
  }, [value])

  return (
    <Plate editor={editor} readOnly>
      <PlateContent
        readOnly
        tabIndex={-1}
        className={cn('agent-prose', className)}
      />
    </Plate>
  )
}
