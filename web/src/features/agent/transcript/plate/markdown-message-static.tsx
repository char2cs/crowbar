import { useMemo } from 'react'
import type { Value } from 'platejs'
import { createStaticEditor, PlateStatic } from 'platejs/static'

import { chatComposerPluginsStatic } from '@/features/agent/composer/plate/chat-composer-plugins'
import { chatMarkdownToValue } from '@/features/agent/composer/plate/chat-composer-serialization'
import { StreamingFenceContext } from '@/features/agent/transcript/plate/streaming-fence'
import { openDiagramFence } from '@/features/agent/transcript/plate/streaming-markdown-parse'
import { cn } from '@/lib/utils'

interface MarkdownMessageStaticProps {
  /** The message's markdown. */
  children: string
  /** A transcript message: its diagram fences render as diagrams. */
  diagrams?: boolean
  className?: string
}

/** Parsed values by markdown, module-level: the virtualized transcript remounts a
 *  row each time it scrolls back in, and re-parsing every one on the main thread
 *  dropped scrolling from 120fps to the 60s. */
const parsedValueCache = new Map<string, Value>()

/** A deep copy, never the cached nodes: `createStaticEditor` can assign ids in place. */
function parsedValue(md: string): Value {
  const cached = parsedValueCache.get(md)
  if (cached) return structuredClone(cached)
  const value = chatMarkdownToValue(md)
  parsedValueCache.set(md, value)
  return structuredClone(value)
}

/** One editor for every row: resolving the plugin set costs ~3ms per `createStaticEditor`.
 *  `PlateStatic` installs its `value` and renders the subtree before the next row starts. */
let staticEditor: ReturnType<typeof createStaticEditor> | null = null
function sharedStaticEditor() {
  return (staticEditor ??= createStaticEditor({ plugins: chatComposerPluginsStatic }))
}

/** Slate's editable keeps a soft line break as a break; so must the settled
 *  copy, or a reply's lines join into one paragraph once it settles. */
const EDITABLE_WHITESPACE = { whiteSpace: 'pre-wrap' } as const

/** A settled message on the same static plugin set, without the interactive
 *  editor's machinery a message that no longer grows has no use for. */
export function MarkdownMessageStatic({
  children,
  diagrams = false,
  className,
}: MarkdownMessageStaticProps) {
  const value = useMemo(() => parsedValue(children), [children])
  // remark parses an unclosed fence as code running to EOF; it renders as code.
  const openFence = useMemo(
    () => (diagrams ? openDiagramFence(children, 0) : null),
    [diagrams, children],
  )
  const fence = useMemo(() => (diagrams ? { live: false, openFence } : null), [diagrams, openFence])

  return (
    <StreamingFenceContext.Provider value={fence}>
      <PlateStatic
        editor={sharedStaticEditor()}
        value={value}
        style={EDITABLE_WHITESPACE}
        className={cn('agent-prose', className)}
      />
    </StreamingFenceContext.Provider>
  )
}
