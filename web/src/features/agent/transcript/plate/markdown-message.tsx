import { useMemo } from 'react'
import { Plate, PlateContent, usePlateEditor } from 'platejs/react'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'
import { chatMarkdownToValue } from '@/features/agent/composer/plate/chat-composer-serialization'
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
  // on every token and this is its hot path.
  const value = useMemo(() => chatMarkdownToValue(children), [children])
  const editor = usePlateEditor({ plugins: chatComposerPlugins, value }, [value])

  return (
    <Plate editor={editor} readOnly>
      <PlateContent
        readOnly
        // Not focusable: a transcript is read, and a tab stop per message would
        // make the keyboard walk the whole conversation to reach the composer.
        tabIndex={-1}
        className={cn('agent-prose', className)}
      />
    </Plate>
  )
}
