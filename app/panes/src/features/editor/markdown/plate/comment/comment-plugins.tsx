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
import remarkGfm from 'remark-gfm'

import { BasicNodesKit } from '@/components/editor/plugins/basic-nodes-kit'
import { CalloutKit } from '@/components/editor/plugins/callout-kit'
import { LinkKit } from '@/components/editor/plugins/link-kit'
import { ListKit } from '@/components/editor/plugins/list-kit'
import { MarkFloatingToolbarKit } from '@/components/editor/plugins/mark-floating-toolbar-kit'

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

// The review comment editor's plugins. Register everything a comment can contain (a missing plugin
// drops that node on edit; HtmlKit carries any raw HTML), but no page-editor furniture: no MathKit
// (katex), SlashKit (reviewers type `/`), BlockMenuKit, BlockPlaceholderKit, or mermaid rendering.
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
  ...MarkFloatingToolbarKit,
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
