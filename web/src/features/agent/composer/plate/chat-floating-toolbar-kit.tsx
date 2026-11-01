'use client'

import {
  CodeIcon,
  TextBIcon,
  TextItalicIcon,
  TextStrikethroughIcon,
  TextUnderlineIcon,
} from '@phosphor-icons/react'
import { KEYS } from 'platejs'
import { createPlatePlugin } from 'platejs/react'

import { FloatingToolbar } from '@/components/ui/floating-toolbar'
import { LinkToolbarButton } from '@/components/ui/link-toolbar-button'
import { MarkToolbarButton } from '@/components/ui/mark-toolbar-button'
import { ToolbarGroup } from '@/components/ui/toolbar'

/** No equation button: `chatComposerPlugins` has no `MathKit`, so the serializer
 *  would have nowhere to put the node. */
export const ChatFloatingToolbarKit = [
  createPlatePlugin({
    key: 'chat-floating-toolbar',
    render: {
      afterEditable: () => (
        <FloatingToolbar>
          <ToolbarGroup>
            <MarkToolbarButton nodeType={KEYS.bold} tooltip="Bold (⌘+B)">
              <TextBIcon />
            </MarkToolbarButton>
            <MarkToolbarButton nodeType={KEYS.italic} tooltip="Italic (⌘+I)">
              <TextItalicIcon />
            </MarkToolbarButton>
            <MarkToolbarButton nodeType={KEYS.underline} tooltip="Underline (⌘+U)">
              <TextUnderlineIcon />
            </MarkToolbarButton>
            <MarkToolbarButton nodeType={KEYS.strikethrough} tooltip="Strikethrough (⌘+⇧+M)">
              <TextStrikethroughIcon />
            </MarkToolbarButton>
            <MarkToolbarButton nodeType={KEYS.code} tooltip="Code (⌘+E)">
              <CodeIcon />
            </MarkToolbarButton>
            <LinkToolbarButton />
          </ToolbarGroup>
        </FloatingToolbar>
      ),
    },
  }),
]
