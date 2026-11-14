// From the Plate registry (`https://platejs.org/r/block-placeholder-kit.json`),
// near-verbatim — the empty-line "Type '/' for commands" hint. Text updated
// to mention the `/` trigger now that `SlashKit` is installed alongside it.
'use client'

import { KEYS } from 'platejs'
import { BlockPlaceholderPlugin } from 'platejs/react'

export const BlockPlaceholderKit = [
  BlockPlaceholderPlugin.configure({
    options: {
      className:
        'before:absolute before:cursor-text before:text-muted-foreground/80 before:content-[attr(placeholder)]',
      placeholders: {
        [KEYS.p]: "Type '/' for commands...",
      },
      query: ({ path }) => path.length === 1,
    },
  }),
]
