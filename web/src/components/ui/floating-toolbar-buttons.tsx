// Adapted from the Plate registry (`https://platejs.org/r/floating-toolbar-kit.json`
// -> `floating-toolbar-buttons`).
//
// TRIMMED HARD: upstream's default button row is "Ask AI" + Turn-into +
// marks + equation + link + Comment + Suggestion + More(sup/sub/kbd), and
// pulls in `@platejs/ai`, `@platejs/comment`, `@platejs/suggestion` plus two
// bespoke Radix dropdown menus (Turn-into, More) that would need hand-built
// primitives of their own (this app's `dropdown-menu.tsx` is base-ui, not
// Radix — the same collision `block-context-menu.tsx` hit). None of AI /
// comment / suggestion are installed features in this app, and Turn-into /
// More aren't required by the task ("selection formatting toolbar" — Turn
// into is already reachable from the slash menu and the block context menu's
// own submenu). What's left: bold/italic/underline/strikethrough/code marks,
// inline equation, and link — a complete formatting toolbar, zero new
// dropdown primitives needed.
'use client'

import { BoldIcon, Code2Icon, ItalicIcon, StrikethroughIcon, UnderlineIcon } from 'lucide-react'
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

      <InlineEquationToolbarButton />

      <LinkToolbarButton />
    </ToolbarGroup>
  )
}
