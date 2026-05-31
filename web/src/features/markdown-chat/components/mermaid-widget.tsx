// web/src/features/markdown-chat/components/mermaid-widget.tsx
import { useEffect, useRef, useState } from 'react'
import mermaid from 'mermaid'
import { registerWidget } from '../extensions/widget-registry'
import type { WidgetComponentProps } from '../extensions/widget-registry'

mermaid.initialize({ startOnLoad: false, theme: 'neutral' })

let mermaidCounter = 0

export function MermaidWidget({ data }: WidgetComponentProps) {
  const source = typeof data.payload === 'string' ? data.payload : ''
  const [svg, setSvg] = useState<string>('')
  const [error, setError] = useState<string>('')
  const idRef = useRef(`mermaid-${++mermaidCounter}`)

  useEffect(() => {
    if (!source.trim()) return
    mermaid
      .render(idRef.current, source)
      .then(({ svg: rendered }) => {
        setSvg(rendered)
        setError('')
      })
      .catch((err: Error) => {
        setError(err.message)
      })
  }, [source])

  if (error) {
    return (
      <div className="rounded border border-destructive/30 bg-destructive/10 p-2 text-xs text-destructive">
        Mermaid error: {error}
      </div>
    )
  }

  if (!svg) {
    return (
      <div className="h-16 animate-pulse rounded border border-border bg-muted" />
    )
  }

  return (
    <div
      className="overflow-auto rounded border border-border bg-card p-2"
      // Safe: Mermaid generates SVG from its own rendering engine, not from user HTML
      dangerouslySetInnerHTML={{ __html: svg }}
    />
  )
}

// Self-register into the widget registry on import
registerWidget('mermaid', MermaidWidget)
