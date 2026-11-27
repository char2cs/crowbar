import { type ReactNode, useState } from 'react'
import { Button } from '@/components/ui/button'
import { MermaidDiagram, type MermaidRenderState } from './mermaid-diagram'

interface MermaidCodeBlockProps {
  code: string
  children: ReactNode
}

export function MermaidCodeBlock({ code, children }: MermaidCodeBlockProps) {
  const [renderState, setRenderState] = useState<MermaidRenderState>('pending')
  const [sourceOpen, setSourceOpen] = useState(false)
  const showSource = sourceOpen || renderState !== 'success'

  return (
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
