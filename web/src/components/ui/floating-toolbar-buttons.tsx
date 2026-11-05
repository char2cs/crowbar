// Adapted from the Plate registry (floating-toolbar-kit.json -> `floating-toolbar-buttons`),
// trimmed to marks, inline equation and link: AI/comment/suggestion aren't installed, and Turn-into
// (reachable from the slash and block menus) and More need Radix menus we don't have.
'use client'

import {
  CodeIcon,
  TextBIcon,
  TextItalicIcon,
  TextStrikethroughIcon,
  TextUnderlineIcon,
} from '@phosphor-icons/react'
import { KEYS } from 'platejs'
import { useEditorReadOnly } from 'platejs/react'

import { InlineEquationToolbarButton } from './equation-toolbar-button'
import { LinkToolbarButton } from './link-toolbar-button'
import { MarkToolbarButton } from './mark-toolbar-button'
import { ToolbarGroup } from './toolbar'

export function FloatingToolbarButtons() {
  const readOnly = useEditorReadOnly()

  if (readOnly) return null

  return (
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

      <InlineEquationToolbarButton />

      <LinkToolbarButton />
    </ToolbarGroup>
  )
}
