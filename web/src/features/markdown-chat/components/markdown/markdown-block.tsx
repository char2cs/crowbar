import { Suspense } from 'react'
import type { WidgetData } from '../../types'
import { parseBlockInfo } from '../../lib/parse-block-info'
import { findBlock } from '../../lib/block-registry'
import { BlockErrorBoundary } from './block-error-boundary'
import { CodeBlockView } from './blocks/code-block-view'

interface MarkdownBlockProps {
  /** Raw fence info string (text after the opening ```). */
  info: string
  /** Fence body. */
  source: string
  /** Turn's widgets, used to resolve `referenced` block payloads by widget-id. */
  widgets?: WidgetData[]
  /** True while the enclosing turn is still streaming. */
  streaming?: boolean
  /** Payload change handler for editable blocks (input only). */
  onChange?: (widgetId: string, payload: unknown) => void
}

// Dispatches a fenced block to its registered extension. Unknown types fall back
// to preformatted text. Each block is wrapped in an error boundary + Suspense so
// heavy/lazy renderers (mermaid, excalidraw) load and fail in isolation.
export function MarkdownBlock({
  info,
  source,
  widgets,
  streaming,
  onChange,
}: MarkdownBlockProps) {
  const parsed = parseBlockInfo(info)
  const ext = findBlock(parsed)

  // No registered extension → it's a plain code block (the default renderer).
  if (!ext) {
    return (
      <CodeBlockView
        type={parsed.type}
        params={parsed.params}
        source={source}
        streaming={streaming}
      />
    )
  }

  const widgetId = parsed.params['widget-id']
  const widget =
    ext.storage === 'referenced' && widgetId
      ? widgets?.find((w) => w.id === widgetId)
      : undefined

  const View = ext.View
  return (
    <BlockErrorBoundary label={ext.type}>
      <Suspense
        fallback={
          <div className="my-1 h-16 animate-pulse rounded border border-border bg-muted" />
        }
      >
        <View
          type={parsed.type}
          params={parsed.params}
          source={source}
          widget={widget}
          streaming={streaming}
          onChange={
            widgetId && onChange ? (p) => onChange(widgetId, p) : undefined
          }
        />
      </Suspense>
    </BlockErrorBoundary>
  )
}
