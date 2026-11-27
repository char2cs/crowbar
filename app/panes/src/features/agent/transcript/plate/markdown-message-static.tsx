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
  children: string
  diagrams?: boolean
  variant?: 'chat' | 'document'
  className?: string
}

const parsedValueCache = new Map<string, Value>()

function parsedValue(md: string): Value {
  const cached = parsedValueCache.get(md)
  if (cached) return structuredClone(cached)
  const value = chatMarkdownToValue(md)
  parsedValueCache.set(md, value)
  return structuredClone(value)
}

let staticEditor: ReturnType<typeof createStaticEditor> | null = null
function sharedStaticEditor() {
  return (staticEditor ??= createStaticEditor({ plugins: chatComposerPluginsStatic }))
}

export function MarkdownMessageStatic({
  children,
  diagrams = false,
  variant = 'document',
  className,
}: MarkdownMessageStaticProps) {
  const grammarsReady = useSyncExternalStore(onGrammarReady, () => readyLanguageIds().length)
  // eslint-disable-next-line react-hooks/exhaustive-deps -- grammarsReady: a new copy per landed grammar, so its blocks decorate again
  const value = useMemo(() => parsedValue(children), [children, grammarsReady])
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
