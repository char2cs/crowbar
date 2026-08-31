'use client'

import { BoldIcon, Code2Icon, ItalicIcon, StrikethroughIcon, UnderlineIcon } from 'lucide-react'
import { KEYS } from 'platejs'
import { createPlatePlugin } from 'platejs/react'

import { FloatingToolbar } from '@/components/ui/floating-toolbar'
import { LinkToolbarButton } from '@/components/ui/link-toolbar-button'
import { MarkToolbarButton } from '@/components/ui/mark-toolbar-button'
import { ToolbarGroup } from '@/components/ui/toolbar'

/**
 * The chat's own selection formatting toolbar — same shell and button
 * primitives as `FloatingToolbarKit`/`CommentFloatingToolbarKit`, minus the
 * inline-equation button neither of those needs here either: `chatComposerPlugins`
 * doesn't register `MathKit` (see that file's own doc comment), so an equation
 * button would insert a node type the markdown serializer has nowhere to put.
 *
 * Only registered where selecting text is possible with a mouse — the composer
 * pill and the empty-document surface both use `chatComposerPlugins`, so this
 * shows up on both without being duplicated.
 */
export const ChatFloatingToolbarKit = [
  createPlatePlugin({
    key: 'chat-floating-toolbar',
    render: {
      afterEditable: () => (
        <FloatingToolbar>
          <ToolbarGroup>
            <MarkToolbarButton nodeType={KEYS.bold} tooltip="Bold (⌘+B)">
              <BoldIcon />
            </MarkToolbarButton>
            <MarkToolbarButton nodeType={KEYS.italic} tooltip="Italic (⌘+I)">
              <ItalicIcon />
            </MarkToolbarButton>
            <MarkToolbarButton nodeType={KEYS.underline} tooltip="Underline (⌘+U)">
              <UnderlineIcon />
            </MarkToolbarButton>
            <MarkToolbarButton nodeType={KEYS.strikethrough} tooltip="Strikethrough (⌘+⇧+M)">
              <StrikethroughIcon />
            </MarkToolbarButton>
            <MarkToolbarButton nodeType={KEYS.code} tooltip="Code (⌘+E)">
              <Code2Icon />
            </MarkToolbarButton>
            <LinkToolbarButton />
          </ToolbarGroup>
        </FloatingToolbar>
      ),
    },
  }),
]
