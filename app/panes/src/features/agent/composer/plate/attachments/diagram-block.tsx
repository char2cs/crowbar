import { useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { Skeleton } from '@/components/ui/skeleton'
import {
  MermaidDiagram,
  type MermaidRenderState,
} from '@/features/editor/markdown/plate/mermaid-diagram'
import type { DiagramFence } from '@/features/agent/transcript/plate/streaming-markdown-parse'

export type DiagramTarget = 'skeleton' | 'diagram' | 'code'

const DIAGRAM_SKELETON_PX = 160
const GROW_MS = 220

function reducedMotion(): boolean {
  return window.matchMedia?.('(prefers-reduced-motion: reduce)').matches === true
}

export function DiagramBlock({
  kind,
  target,
  source,
  preview,
  code,
}: {
  kind: DiagramFence
  target: DiagramTarget
  source: string
  preview?: ReactNode
  code: ReactNode
}) {
  const [rendered, setRendered] = useState(false)
  const mermaid = kind === 'mermaid' && target === 'diagram'
  const shown: DiagramTarget = mermaid && !rendered ? 'skeleton' : target
  const ref = useRef<HTMLDivElement>(null)
  const previous = useRef(shown)

  useLayoutEffect(() => {
    const from = previous.current
    previous.current = shown
    const el = ref.current
    if (from !== 'skeleton' || shown === 'skeleton' || !el || reducedMotion()) return
    if (typeof el.animate !== 'function') return
    const to = el.getBoundingClientRect().height
    // react-doctor-disable-next-line no-layout-property-animation -- one block, once per diagram: the reserved box must grow to its content's real height; a transform would distort the diagram instead
    el.animate([{ height: `${DIAGRAM_SKELETON_PX}px` }, { height: `${to}px` }], {
      duration: GROW_MS,
      easing: 'ease-out',
    })
  }, [shown])

  const onStateChange = (state: MermaidRenderState) => {
    if (state !== 'pending') setRendered(true)
  }

  return (
    <div ref={ref} className="chat-diagram-block my-2 overflow-hidden" data-state={shown}>
      {shown === 'skeleton' && (
        <Skeleton
          contentEditable={false}
          aria-label="Rendering diagram"
          style={{ height: DIAGRAM_SKELETON_PX }}
        />
      )}
      {mermaid && (
        <div contentEditable={false} className={shown === 'skeleton' ? 'hidden' : undefined}>
          <MermaidDiagram code={source} onStateChange={onStateChange} />
        </div>
      )}
      {!mermaid && shown === 'diagram' && (
        <div contentEditable={false} className="select-none">
          {preview}
        </div>
      )}
      <div className={shown === 'code' ? undefined : 'hidden'}>{code}</div>
    </div>
  )
}
