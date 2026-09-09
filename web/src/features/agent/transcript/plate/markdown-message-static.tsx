import { useMemo } from 'react'
import type { Value } from 'platejs'
import { createStaticEditor, PlateStatic } from 'platejs/static'

import { chatComposerPluginsStatic } from '@/features/agent/composer/plate/chat-composer-plugins'
import { chatMarkdownToValue } from '@/features/agent/composer/plate/chat-composer-serialization'
import { cn } from '@/lib/utils'

interface MarkdownMessageStaticProps {
  /** The message's markdown. */
  children: string
  className?: string
}

/** Keyed by the exact markdown parsed, module-level so it survives what
 *  `useMemo` cannot: the virtualized transcript (agent-transcript.tsx)
 *  unmounts a row the instant it scrolls past the overscan window and
 *  mounts a fresh one — with a fresh `useMemo` cache — the instant it
 *  scrolls back in. A brisk scroll back through settled history re-enters
 *  dozens of rows a second, and without this each one re-ran a real
 *  markdown parse (`chatMarkdownToValue` — a full AST parse, not string
 *  work) synchronously in React's commit phase, on the main thread, for
 *  content that had already been parsed once and thrown away. Measured
 *  live as manually scrolling the transcript tanking from 120fps to the
 *  60s. Unbounded on purpose: bounded by the count of distinct message
 *  texts this chat ever rendered, the same order of magnitude as the
 *  `messages` array already held in memory for the whole session. */
const parsedValueCache = new Map<string, Value>()

/** A fresh deep copy every time, never the cached nodes themselves —
 *  `createStaticEditor` normalizes on construction, which can assign node
 *  ids/keys in place, and two rows sharing one mutable node graph is
 *  exactly the id-noise class of bug this codebase already hit once with
 *  Plate. Cheaper than a re-parse regardless: this is JSON-safe plain
 *  objects, not a real parse. */
function parsedValue(md: string): Value {
  const cached = parsedValueCache.get(md)
  if (cached) return structuredClone(cached)
  const value = chatMarkdownToValue(md)
  parsedValueCache.set(md, value)
  return structuredClone(value)
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
        value: parsedValue(children),
      }),
    [children],
  )

  return <PlateStatic editor={editor} className={cn('agent-prose', className)} />
}
