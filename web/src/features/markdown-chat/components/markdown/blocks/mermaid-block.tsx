// web/src/features/markdown-chat/components/markdown/blocks/mermaid-block.tsx
import { lazy } from 'react'
import { registerBlock } from '@/features/markdown-chat/lib/block-registry'

// Light registration only — the mermaid bundle is pulled in lazily by the
// View import the first time a diagram actually renders.
registerBlock({
  type: 'mermaid',
  storage: 'inline',
  match: (info) => info.type === 'mermaid',
  View: lazy(() => import('./mermaid-block-impl')),
})
