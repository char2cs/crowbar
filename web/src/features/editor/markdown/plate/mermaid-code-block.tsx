import { type ReactNode, useState } from 'react'
import { Button } from '@/components/ui/button'
import { MermaidDiagram, type MermaidRenderState } from './mermaid-diagram'

interface MermaidCodeBlockProps {
  code: string
  children: ReactNode
}

/**
 * Wraps a ```mermaid fenced code block's normal (editable) rendering with a
 * diagram preview. `children` is the SAME `<pre><code>` markup an ordinary
 * code block renders (see code-block-node.tsx) — that markup is what
 * Slate/Plate maps its DOM nodes onto, and it stays mounted here
 * unconditionally. Toggling the source view only flips a CSS class
 * (`hidden`), never unmounts it, so Slate's DOM<->node mapping for this
 * block is never disturbed and the source stays genuinely editable at all
 * times, whether or not it's currently visible.
 */
export function MermaidCodeBlock({ code, children }: MermaidCodeBlockProps) {
  const [renderState, setRenderState] = useState<MermaidRenderState>('pending')
  const [sourceOpen, setSourceOpen] = useState(false)
  // The source is always reachable: forced open while pending or on error
  // (so an invalid line is immediately visible to fix — the robustness
  // requirement), or whenever the user has pinned it open. It only
  // auto-collapses once a diagram has actually rendered successfully.
  const showSource = sourceOpen || renderState !== 'success'

  return (
    // No `contentEditable` here: this wrapper inherits the ambient editable
    // state of its Slate-managed ancestor (same as CodeBlockElement's own
    // `relative rounded-md bg-muted/50` wrapper never sets it either) — only
    // the non-editable islands below (the diagram box, the toggle button)
    // opt out, exactly like that component's toolbar does.
    <div className="mermaid-code-block">
      <div
        contentEditable={false}
        className="mb-2 select-none rounded-md border border-border bg-background/60 p-2"
      >
        <MermaidDiagram code={code} onStateChange={setRenderState} />
      </div>
      {renderState === 'success' && (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          contentEditable={false}
          className="mb-1 h-6 select-none px-2 text-muted-foreground text-xs"
          onClick={() => setSourceOpen((open) => !open)}
        >
          {sourceOpen ? 'Hide source' : 'Edit source'}
        </Button>
      )}
      <div className={showSource ? undefined : 'hidden'}>{children}</div>
    </div>
  )
}
