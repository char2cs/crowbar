import { useMemo, useSyncExternalStore } from 'react'
import type { Value } from 'platejs'
import { createStaticEditor, PlateStatic } from 'platejs/static'

import { chatComposerPluginsStatic } from '@/features/agent/composer/plate/chat-composer-plugins'
import { chatMarkdownToValue } from '@/features/agent/composer/plate/chat-composer-serialization'
import { CHAT_PROSE_STYLE } from '@/features/agent/transcript/plate/chat-prose-style'
import { StreamingFenceContext } from '@/features/agent/transcript/plate/streaming-fence'
import { openDiagramFence } from '@/features/agent/transcript/plate/streaming-markdown-parse'
import { onGrammarReady, readyLanguageIds } from '@/features/editor/lib/shiki-grammars'
import { cn } from '@/lib/utils'

interface MarkdownMessageStaticProps {
  /** The message's markdown. */
  children: string
  /** A transcript message: its diagram fences render as diagrams. */
  diagrams?: boolean
  /** `chat`: a message, whose line breaks stand as the streaming renderer drew
   *  them; a `document` (the default) reads as CommonMark. */
  variant?: 'chat' | 'document'
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

/** A settled message on the same static plugin set, without the interactive
 *  editor's machinery a message that no longer grows has no use for. */
export function MarkdownMessageStatic({
  children,
  diagrams = false,
  variant = 'document',
  className,
}: MarkdownMessageStaticProps) {
  // Code is decorated as it renders; a grammar that lands later needs fresh nodes to decorate.
  const grammarsReady = useSyncExternalStore(onGrammarReady, () => readyLanguageIds().length)
  // eslint-disable-next-line react-hooks/exhaustive-deps -- grammarsReady: a new copy per landed grammar, so its blocks decorate again
  const value = useMemo(() => parsedValue(children), [children, grammarsReady])
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
        style={variant === 'chat' ? CHAT_PROSE_STYLE : undefined}
        className={cn('agent-prose', className)}
      />
    </StreamingFenceContext.Provider>
  )
}
