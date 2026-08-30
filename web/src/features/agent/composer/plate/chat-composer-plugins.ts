import { MarkdownPlugin } from '@platejs/markdown'
import { CodeBlockRules } from '@platejs/code-block'
import { CodeBlockPlugin, CodeLinePlugin } from '@platejs/code-block/react'
import remarkGfm from 'remark-gfm'

import { BasicNodesKit } from '@/components/editor/plugins/basic-nodes-kit'
import { CalloutKit } from '@/components/editor/plugins/callout-kit'
import { IndentPlugin } from '@platejs/indent/react'
import { ListKit } from '@/components/editor/plugins/list-kit'
import { LinkKit } from '@/components/editor/plugins/link-kit'
import {
  TableCellHeaderPlugin,
  TableCellPlugin,
  TablePlugin,
  TableRowPlugin,
} from '@platejs/table/react'
import { HtmlKit } from '@/features/editor/markdown/plate/html-node'
import { MarkdownImageKit } from '@/features/editor/markdown/plate/markdown-image-node'
import { calloutMarkdownRules } from '@/features/editor/markdown/plate/markdown-callout-rules'
import { htmlMarkdownRules } from '@/features/editor/markdown/plate/markdown-html-rules'
import { ChatFreshTextPlugin } from '@/features/agent/transcript/plate/chat-fresh-text-plugin'
import {
  CommentCodeBlockElement,
  CommentCodeLineElement,
  CommentTableCellElement,
  CommentTableCellHeaderElement,
  CommentTableElement,
  CommentTableRowElement,
} from '@/features/editor/markdown/plate/comment/comment-nodes'

/**
 * The chat's markdown, both directions.
 *
 * ONE set for what a person writes and what an agent answers, because they are
 * the same conversation: a table pasted into a prompt and a table in a reply
 * have to mean the same thing, and two sets would let them drift.
 *
 * What sizes it is the "a plugin set decides what SURVIVES" rule. `@platejs/
 * markdown` DROPS any node whose plugin is unregistered — so an omission here is
 * not a missing affordance, it is silent loss: a table pasted into the box would
 * vanish from the prompt actually sent, and a table in an answer would vanish
 * from the transcript. Hence tables, callouts, images and fenced code are all
 * registered even though nobody types most of them. `HtmlKit` is the backstop
 * for everything else.
 *
 * What is left OUT is interaction furniture, not node types:
 *
 * - **`SlashKit` — and this one is not a size decision.** `/` in an agent chat
 *   opens CROWBAR'S skill picker, which lists what the provider will answer to.
 *   Registering Plate's slash menu would put two menus on one key, and the one
 *   that won would be the one that knows nothing about the provider.
 * - `MathKit` — katex is ~280 KB of library and stylesheet, imported at module
 *   scope. A prompt that types `$x$` means it literally, and the agent reads the
 *   characters either way.
 * - `BlockMenuKit`, `BlockPlaceholderKit`, `FloatingToolbarKit` — drag handles,
 *   a "type / for commands" hint that would be a lie here, and a selection
 *   toolbar. Page-editor furniture on a box that is usually one line; the
 *   markdown input rules already produce every block this set registers.
 *
 * Table and code-block nodes use the COMMENT editor's minimal components rather
 * than the file editor's, which pull `createLowlight(all)`, cmdk and a Radix
 * popover for affordances a chat has no use for.
 */
export const chatComposerPlugins = [
  ...BasicNodesKit,
  ...ListKit,
  // The canvas indents a list 16px a level; the shared kit's step is 24px, and
  // Plate writes it as an INLINE style — no stylesheet can reach it, so it is
  // set where it is computed. Re-configuring here rather than in `IndentKit`
  // keeps the file editor's own rhythm out of it.
  IndentPlugin.configure({ options: { offset: 16 } }),
  ...LinkKit,
  ...HtmlKit,
  ...CalloutKit,
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
  // Renders the streaming transcript's fade-in. Inert everywhere else: the
  // mark it looks for is set only by streaming-value-patch.ts, so it never
  // fires in the composer or on a recorded, non-streaming message.
  ChatFreshTextPlugin,
  MarkdownPlugin.configure({
    options: {
      remarkPlugins: [remarkGfm],
      // The same punctuation the file and comment editors pin. A prompt is read
      // by a model, not diffed against a file, but a person who writes `- item`
      // in one Crowbar box and sees `* item` in another is being told the two
      // are different editors.
      remarkStringifyOptions: { emphasis: '*', bullet: '-' },
      rules: { ...calloutMarkdownRules, ...htmlMarkdownRules },
    },
  }),
]
