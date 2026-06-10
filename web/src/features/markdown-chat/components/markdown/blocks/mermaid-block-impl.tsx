// web/src/features/markdown-chat/components/markdown/blocks/mermaid-block-impl.tsx
import { useEffect, useRef, useState } from 'react'
import mermaid from 'mermaid'
import type { BlockViewProps } from '@/features/markdown-chat/lib/block-registry'

mermaid.initialize({ startOnLoad: false, theme: 'neutral' })

let mermaidCounter = 0

export default function MermaidBlockView({ source, streaming }: BlockViewProps) {
  const [svg, setSvg] = useState<string>('')
  const [error, setError] = useState<string>('')
  const idRef = useRef(`mermaid-block-${++mermaidCounter}`)

  useEffect(() => {
    // While streaming the diagram source is likely incomplete; don't try to
    // render it (mermaid would throw on every keystroke).
    if (streaming) return
    if (!source.trim()) return
    let cancelled = false
    mermaid
      .render(idRef.current, source)
      .then(({ svg: rendered }) => {
        if (cancelled) return
        setSvg(rendered)
        setError('')
      })
      .catch((err: Error) => {
        if (cancelled) return
        setError(err.message)
      })
    return () => {
      cancelled = true
    }
  }, [source, streaming])

  // Still streaming: show the raw source so the user can watch it arrive.
  if (streaming) {
    return (
      <pre className="overflow-auto rounded border border-border bg-muted p-2 text-xs text-muted-foreground">
        {source}
      </pre>
    )
  }

  if (error) {
    return (
      <div className="rounded border border-destructive/30 bg-destructive/10 p-2 text-xs text-destructive">
        Mermaid error: {error}
      </div>
    )
  }

  if (!svg) {
    return <div className="h-16 animate-pulse rounded border border-border bg-muted" />
  }

  return (
    <div
      className="overflow-auto rounded border border-border bg-card p-2"
      // Safe: Mermaid generates SVG from its own rendering engine, not from user HTML
      dangerouslySetInnerHTML={{ __html: svg }}
    />
  )
}
