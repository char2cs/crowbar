import { useEffect, useId, useRef, useState } from 'react'
import { buildMermaidThemeVariables, useMermaidThemeVersion } from './mermaid-theme'

// Debounced so mermaid's parse and dagre layout don't run per keystroke; looser than the
// markdown editor's 150 ms sink because a layout pass is far heavier than a serialize.
const RENDER_DEBOUNCE_MS = 400

// One module-scoped import promise: every diagram awaits the same load, and a remount can't
// re-fetch or abandon it. Only reachable from here, so the chunk loads only for mermaid blocks.
let mermaidPromise: Promise<typeof import('mermaid')> | null = null
function loadMermaid(): Promise<typeof import('mermaid')> {
  return (mermaidPromise ??= import('mermaid'))
}

export type MermaidRenderState = 'pending' | 'success' | 'error'

interface MermaidDiagramProps {
  code: string
  onStateChange: (state: MermaidRenderState) => void
}

/** Renders mermaid source to SVG. The only importer of `mermaid`, reached only from a mermaid
 *  code block, so other documents never load it; don't hoist the import. Any mermaid throw shows
 *  as an inline message: no ErrorBoundary guards this deep in the live Plate tree. */
export function MermaidDiagram({ code, onStateChange }: MermaidDiagramProps) {
  const reactId = useId()
  // mermaid's `render(id, ...)` uses `id` as a literal SVG element id;
  // useId's colons aren't valid there.
  const elementId = `mermaid-diagram-${reactId.replace(/[^a-zA-Z0-9_-]/g, '')}`
  const themeVersion = useMermaidThemeVersion()
  const [svg, setSvg] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const requestIdRef = useRef(0)
  // `onStateChange` via ref so a new callback identity from a parent re-render
  // can't retrigger the effect (and tear down its in-flight render).
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
    // Start the cached load synchronously, outside the debounce: re-renders (selection changes,
    // StrictMode) clear the timer, and a load started inside it might never run.
    const mermaidP = loadMermaid()
    const timer = setTimeout(() => {
      void mermaidP
        .then(async ({ default: mermaid }) => {
          if (cancelled || requestIdRef.current !== requestId) return
          mermaid.initialize({
            startOnLoad: false,
            securityLevel: 'strict',
            // Otherwise a failed render leaves mermaid's "Syntax error" graphic
            // appended to document.body; our own error card below replaces it.
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
    // themeVersion is read only to force a re-render (and therefore a fresh
    // mermaid.render with current CSS-var colors) when the app's light/dark
    // class flips — it isn't otherwise used in the body below.
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
      // mermaid's `securityLevel: 'strict'` sanitizes the markup itself
      // (DOMPurify) before this string is produced — this mirrors mermaid's
      // own documented usage (`element.innerHTML = svg`).
      dangerouslySetInnerHTML={{ __html: svg }}
    />
  )
}
