import { type ReactNode, useState } from 'react'
import { Button } from '@/components/ui/button'
import { MermaidDiagram, type MermaidRenderState } from './mermaid-diagram'

interface MermaidCodeBlockProps {
  code: string
  children: ReactNode
}

/** A ```mermaid block's editable rendering plus a diagram preview. `children` (the code block's
 *  `<pre><code>`, which Slate maps its DOM onto) stays mounted always; hiding the source only
 *  toggles a class, so it remains editable. */
export function MermaidCodeBlock({ code, children }: MermaidCodeBlockProps) {
  const [renderState, setRenderState] = useState<MermaidRenderState>('pending')
  const [sourceOpen, setSourceOpen] = useState(false)
  // The source shows while pending or on error (so a bad line is in view to fix) or when pinned
  // open; it collapses only once a diagram renders.
  const showSource = sourceOpen || renderState !== 'success'

  return (
    // No `contentEditable` here: the wrapper inherits its Slate ancestor's editable state; only
    // the non-editable islands below (diagram, toggle) opt out, as CodeBlockElement's toolbar does.
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
