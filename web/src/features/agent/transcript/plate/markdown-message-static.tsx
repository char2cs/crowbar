import { useMemo } from 'react'
import { createStaticEditor, PlateStatic } from 'platejs/static'

import { chatComposerPluginsStatic } from '@/features/agent/composer/plate/chat-composer-plugins'
import { chatMarkdownToValue } from '@/features/agent/composer/plate/chat-composer-serialization'
import { cn } from '@/lib/utils'

interface MarkdownMessageStaticProps {
  /** The message's markdown. */
  children: string
  className?: string
}

/**
 * A settled message, rendered without an interactive editor.
 *
 * Same `chatComposerPluginsStatic` set MarkdownMessage's interactive editor
 * uses (just Link/Callout swapped for static-safe variants — see
 * chat-composer-plugins.ts) so a table, callout or code span still has
 * exactly one appearance. What this drops is everything MarkdownMessage
 * exists for: `usePlateEditor`'s Slate machinery, `applyStreamedValue`'s
 * patch-in-place, the fade-in plugin's mark. None of that has a reason to
 * exist once a message is no longer growing — see message-row.tsx for how
 * only the one still-streaming bubble gets the interactive path instead.
 */
export function MarkdownMessageStatic({ children, className }: MarkdownMessageStaticProps) {
  const editor = useMemo(
    () =>
      createStaticEditor({
        plugins: chatComposerPluginsStatic,
        value: chatMarkdownToValue(children),
      }),
    [children],
  )

  return <PlateStatic editor={editor} className={cn('agent-prose', className)} />
}
