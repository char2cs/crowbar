import { useEffect, useId, useRef, useState } from 'react'
import { buildMermaidThemeVariables, useMermaidThemeVersion } from './mermaid-theme'

const RENDER_DEBOUNCE_MS = 400

let mermaidPromise: Promise<typeof import('mermaid')> | null = null
function loadMermaid(): Promise<typeof import('mermaid')> {
  return (mermaidPromise ??= import('mermaid'))
}

export type MermaidRenderState = 'pending' | 'success' | 'error'

interface MermaidDiagramProps {
  code: string
  onStateChange: (state: MermaidRenderState) => void
}

export function MermaidDiagram({ code, onStateChange }: MermaidDiagramProps) {
  const reactId = useId()
  const elementId = `mermaid-diagram-${reactId.replace(/[^a-zA-Z0-9_-]/g, '')}`
  const themeVersion = useMermaidThemeVersion()
  const [svg, setSvg] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const requestIdRef = useRef(0)
  const onStateChangeRef = useRef(onStateChange)
  onStateChangeRef.current = onStateChange

  useEffect(() => {
    const trimmed = code.trim()
    const requestId = ++requestIdRef.current

    if (!trimmed) {
      setSvg(null)
      setError(null)
      onStateChangeRef.current('pending')
      return
    }

    onStateChangeRef.current('pending')
    let cancelled = false
    const mermaidP = loadMermaid()
    const timer = setTimeout(() => {
      void mermaidP
        .then(async ({ default: mermaid }) => {
          if (cancelled || requestIdRef.current !== requestId) return
          mermaid.initialize({
            startOnLoad: false,
            securityLevel: 'strict',
            suppressErrorRendering: true,
            theme: 'base',
            themeVariables: buildMermaidThemeVariables(),
          })
          const { svg: rendered } = await mermaid.render(elementId, trimmed)
          if (cancelled || requestIdRef.current !== requestId) return
          setSvg(rendered)
          setError(null)
          onStateChangeRef.current('success')
        })
        .catch((err: unknown) => {
          if (cancelled || requestIdRef.current !== requestId) return
          setSvg(null)
          setError(err instanceof Error ? err.message : String(err))
          onStateChangeRef.current('error')
        })
    }, RENDER_DEBOUNCE_MS)

    return () => {
      cancelled = true
      clearTimeout(timer)
    }
  }, [code, elementId, themeVersion])

  if (error) {
    return (
      <div className="rounded-sm border border-destructive/30 bg-destructive/5 p-3 text-xs text-destructive">
        <span className="font-medium">Couldn't render this diagram:</span> {error}
      </div>
    )
  }

  if (!svg) {
    return <div className="p-3 text-xs text-muted-foreground">Rendering diagram…</div>
  }

  return (
    <div
      className="mermaid-diagram flex justify-center overflow-auto [&_svg]:h-auto [&_svg]:max-w-full"
      dangerouslySetInnerHTML={{ __html: svg }}
    />
  )
}
