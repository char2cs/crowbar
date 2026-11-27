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
