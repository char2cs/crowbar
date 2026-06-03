// web/src/features/markdown-chat/components/markdown/blocks/excalidraw-block.tsx
import { lazy } from 'react'
import { registerBlock } from '@/features/markdown-chat/lib/block-registry'

// Light registration file — no static Excalidraw import so the heavy editor
// bundle is only loaded when an excalidraw block is actually rendered.
registerBlock({
  type: 'excalidraw',
  storage: 'referenced',
  match: (info) => info.type === 'excalidraw',
  View: lazy(() =>
    import('./excalidraw-block-impl').then((m) => ({ default: m.ExcalidrawView })),
  ),
  Editor: lazy(() =>
    import('./excalidraw-block-impl').then((m) => ({ default: m.ExcalidrawEditor })),
  ),
  createPayload: () => null,
})
