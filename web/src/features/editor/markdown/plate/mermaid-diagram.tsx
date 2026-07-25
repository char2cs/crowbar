import { useEffect, useId, useRef, useState } from 'react'
import { buildMermaidThemeVariables, useMermaidThemeVersion } from './mermaid-theme'

// Debounced so a diagram doesn't re-run mermaid's parse + dagre layout on
// every keystroke while the user is mid-edit. Looser than the markdown
// editor's own SINK_DELAY_MS (150ms in markdown-editor-pane.tsx) because a
// mermaid layout pass is far heavier than a markdown serialize.
const RENDER_DEBOUNCE_MS = 400

// Import `mermaid` ONCE for the whole app and cache the promise. Mermaid is a
// large lazy dependency (kept out of the entry chunk — see the note below); a
// per-component `import()` re-fetched it on every mount and, worse, could be
// torn down mid-flight. Caching the promise at module scope means the first
// diagram to mount triggers the load and every diagram — this mount or a later
// one — awaits the same result. `mermaid` is only reachable from here, so the
// chunk still loads only when a document actually contains a mermaid block.
let mermaidPromise: Promise<typeof import('mermaid')> | null = null
function loadMermaid(): Promise<typeof import('mermaid')> {
  return (mermaidPromise ??= import('mermaid'))
}

export type MermaidRenderState = 'pending' | 'success' | 'error'

interface MermaidDiagramProps {
  code: string
  onStateChange: (state: MermaidRenderState) => void
}

/**
 * Renders mermaid source to an SVG diagram. `mermaid` is dynamically
 * imported here and ONLY here — this component is reached exclusively from
 * a ```mermaid code block actually present in the open document (see
 * mermaid-code-block.tsx / code-block-node.tsx), so opening any other
 * markdown file never pays for the dependency. Do not hoist this import;
 * `bunx vite build` + a grep of the entry chunk for `mermaid` is the
 * standing gate on that (mirrors the katex/platejs gate noted in
 * markdown-plugins.ts).
 *
 * Invalid syntax — or `mermaid` throwing for any other reason — is caught
 * and rendered as a quiet inline message rather than propagating: this
 * component lives deep inside the live Plate tree, past the point the
 * ErrorBoundary in markdown-editor-pane.tsx can catch a render throw (that
 * boundary only guards deserialize/construction), so a bad diagram must
 * never be able to take the whole editor down with it.
 */
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
    // Kick the (module-cached) load off SYNCHRONOUSLY, outside the debounce:
    // the old code only called `import('mermaid')` from inside the 400ms
    // setTimeout, so any re-render/remount within that window (Plate re-renders
    // the code block on selection changes; StrictMode double-invokes) cleared
    // the timer before it fired and the chunk NEVER loaded. Starting the import
    // here — cached across every instance — makes the load unkillable; only the
    // (fast, once mermaid is loaded) render pass stays debounced.
    const mermaidP = loadMermaid()
    const timer = setTimeout(() => {
      void mermaidP
        .then(async ({ default: mermaid }) => {
          if (cancelled || requestIdRef.current !== requestId) return
          mermaid.initialize({
            startOnLoad: false,
            securityLevel: 'strict',
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
