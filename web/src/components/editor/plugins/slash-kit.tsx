// From the Plate registry (`https://platejs.org/r/slash-kit.json`), verbatim —
// no primitive collision.
'use client'

import { SlashInputPlugin, SlashPlugin } from '@platejs/slash-command/react'
import { type SlateEditor, KEYS } from 'platejs'

import { SlashInputElement } from '@/components/ui/slash-node'

export const SlashKit = [
  SlashPlugin.configure({
    options: {
      triggerQuery: (editor: SlateEditor) =>
        !editor.api.some({
          match: { type: editor.getType(KEYS.codeBlock) },
        }),
    },
  }),
  SlashInputPlugin.withComponent(SlashInputElement),
]
