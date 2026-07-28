'use client'

import { MarkdownPlugin } from '@platejs/markdown'
import { CodeBlockRules } from '@platejs/code-block'
import { CodeBlockPlugin, CodeLinePlugin } from '@platejs/code-block/react'
import {
  TableCellHeaderPlugin,
  TableCellPlugin,
  TablePlugin,
  TableRowPlugin,
} from '@platejs/table/react'
import { BoldIcon, Code2Icon, ItalicIcon, StrikethroughIcon, UnderlineIcon } from 'lucide-react'
import { KEYS } from 'platejs'
import { createPlatePlugin } from 'platejs/react'
import remarkGfm from 'remark-gfm'

import { BasicNodesKit } from '@/components/editor/plugins/basic-nodes-kit'
import { CalloutKit } from '@/components/editor/plugins/callout-kit'
import { LinkKit } from '@/components/editor/plugins/link-kit'
import { ListKit } from '@/components/editor/plugins/list-kit'
import { FloatingToolbar } from '@/components/ui/floating-toolbar'
import { LinkToolbarButton } from '@/components/ui/link-toolbar-button'
import { MarkToolbarButton } from '@/components/ui/mark-toolbar-button'
import { ToolbarGroup } from '@/components/ui/toolbar'

import { calloutMarkdownRules } from '../markdown-callout-rules'
import { htmlMarkdownRules } from '../markdown-html-rules'
import { HtmlKit } from '../html-node'
import { MarkdownImageKit } from '../markdown-image-node'
import {
  CommentCodeBlockElement,
  CommentCodeLineElement,
  CommentTableCellElement,
  CommentTableCellHeaderElement,
  CommentTableElement,
  CommentTableRowElement,
} from './comment-nodes'

// The selection formatting toolbar, minus the inline-equation button the full
// editor's `FloatingToolbarButtons` carries. That button is why this is a
// separate composition rather than a reuse: it imports @platejs/math, which
// imports katex at module scope (~280 KB), for a control this editor has no
// node type to insert into. The positioning shell and the button primitives
// ARE reused — only the button list differs.
const CommentFloatingToolbarKit = [
  createPlatePlugin({
    key: 'comment-floating-toolbar',
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

/**
 * The plugin set behind the review comment editor.
 *
 * Sized by two rules, in this order:
 *
 * 1. **Register whatever a comment can CONTAIN.** @platejs/markdown drops a
 *    node whose plugin isn't registered, and a comment is deserialized every
 *    time someone edits one. So an omitted plugin is not a missing affordance,
 *    it is silent data loss on edit — the table, the image, the alert block
 *    disappears from a comment its author only meant to fix a typo in.
 *    `HtmlKit` is the backstop here: it carries any raw HTML block through
 *    verbatim, so even constructs nothing else models survive.
 *
 * 2. **Buy no page-editor furniture.** Everything above is about the DOCUMENT
 *    model. The interaction layer is cut to what a two-sentence box needs:
 *    marks and links in a selection toolbar, markdown input rules for
 *    everything else.
 *
 * Deliberately absent, each for a reason worth stating:
 *
 * - `MathKit` — katex, ~280 KB of stylesheet and library, so a review comment
 *   can typeset an integral. `remarkMath` is dropped with it, which leaves
 *   `$x$` as the literal text it already was.
 * - `SlashKit` — the `/` menu is a long-form-document affordance. Markdown
 *   input rules already produce every block this set registers, and `/` is a
 *   character code reviewers type constantly (paths).
 * - `BlockMenuKit` — drag handles and a block context menu on a 72px box.
 * - `BlockPlaceholderKit` — its hint reads "Type '/' for commands", and there
 *   is no slash menu here. The composer passes a real placeholder instead.
 * - mermaid rendering — the fence still round-trips as a fenced block, so
 *   nothing is lost from the comment; it just doesn't draw the diagram while
 *   you type. `MarkdownPreview` doesn't either.
 */
export const commentPlugins = [
  ...BasicNodesKit,
  ...ListKit,
  ...LinkKit,
  ...CalloutKit,
  ...HtmlKit,
  ...MarkdownImageKit,
  TablePlugin.withComponent(CommentTableElement),
  TableRowPlugin.withComponent(CommentTableRowElement),
  TableCellPlugin.withComponent(CommentTableCellElement),
  TableCellHeaderPlugin.withComponent(CommentTableCellHeaderElement),
  CodeBlockPlugin.configure({
    inputRules: [CodeBlockRules.markdown({ on: 'match' })],
    node: { component: CommentCodeBlockElement },
    shortcuts: { toggle: { keys: 'mod+alt+8' } },
  }),
  CodeLinePlugin.withComponent(CommentCodeLineElement),
  ...CommentFloatingToolbarKit,
  MarkdownPlugin.configure({
    options: {
      remarkPlugins: [remarkGfm],
      // Same punctuation the file editor pins (see markdown-plugins.ts): both
      // write to the same kind of storage and a comment moved between them
      // should not be re-punctuated.
      remarkStringifyOptions: { emphasis: '*', bullet: '-' },
      rules: { ...calloutMarkdownRules, ...htmlMarkdownRules },
    },
  }),
]
