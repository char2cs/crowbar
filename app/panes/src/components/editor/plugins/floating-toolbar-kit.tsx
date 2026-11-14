// From the Plate registry (`https://platejs.org/r/floating-toolbar-kit.json`),
// verbatim — the selection formatting toolbar.
'use client'

import { createPlatePlugin } from 'platejs/react'

import { FloatingToolbar } from '@/components/ui/floating-toolbar'
import { FloatingToolbarButtons } from '@/components/ui/floating-toolbar-buttons'

export const FloatingToolbarKit = [
  createPlatePlugin({
    key: 'floating-toolbar',
    render: {
      afterEditable: () => (
        <FloatingToolbar>
          <FloatingToolbarButtons />
        </FloatingToolbar>
      ),
    },
  }),
]
