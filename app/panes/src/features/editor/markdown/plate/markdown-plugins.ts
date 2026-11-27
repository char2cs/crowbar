import { MarkdownPlugin } from '@platejs/markdown'
import remarkGfm from 'remark-gfm'
import remarkMath from 'remark-math'

import 'katex/dist/katex.min.css'

import { BasicNodesKit } from '@/components/editor/plugins/basic-nodes-kit'
import { BlockMenuKit } from '@/components/editor/plugins/block-menu-kit'
import { BlockPlaceholderKit } from '@/components/editor/plugins/block-placeholder-kit'
import { CalloutKit } from '@/components/editor/plugins/callout-kit'
import { CodeBlockKit } from '@/components/editor/plugins/code-block-kit'
import { FloatingToolbarKit } from '@/components/editor/plugins/floating-toolbar-kit'
import { LinkKit } from '@/components/editor/plugins/link-kit'
import { ListKit } from '@/components/editor/plugins/list-kit'
import { MathKit } from '@/components/editor/plugins/math-kit'
import { SlashKit } from '@/components/editor/plugins/slash-kit'
import { TableKit } from '@/components/editor/plugins/table-kit'
import { HtmlKit } from './html-node'
import { calloutMarkdownRules } from './markdown-callout-rules'
import { htmlMarkdownRules } from './markdown-html-rules'
import { softBreakMarkdownRules } from './markdown-softbreak-rules'
import { MarkdownImageKit } from './markdown-image-node'
import { underlineMarkdownRules } from './markdown-underline-rules'

function remarkGfmCompactTables(this: ThisParameterType<typeof remarkGfm>) {
  remarkGfm.call(this, { tablePipeAlign: false })
}

export const markdownPlugins = [
  ...BasicNodesKit,
  ...ListKit,
  ...TableKit,
  ...CodeBlockKit,
  ...LinkKit,
  ...MathKit,
  ...CalloutKit,
  ...HtmlKit,
  ...MarkdownImageKit,
  ...SlashKit,
  ...BlockMenuKit,
  ...FloatingToolbarKit,
  ...BlockPlaceholderKit,
  MarkdownPlugin.configure({
    options: {
      remarkPlugins: [remarkGfmCompactTables, remarkMath],
      remarkStringifyOptions: { emphasis: '*', bullet: '-', rule: '-' },
      rules: {
        ...calloutMarkdownRules,
        ...htmlMarkdownRules,
        ...underlineMarkdownRules,
        ...softBreakMarkdownRules,
      },
    },
  }),
]
