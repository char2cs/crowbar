import { MarkdownPlugin } from '@platejs/markdown'
import remarkGfm from 'remark-gfm'
import remarkMath from 'remark-math'

// KaTeX's stylesheet (math is unstyled spans without it). Reached only via the lazy
// markdown-editor-pane chunk; never hoist it (or MathKit) into an eager import, or KaTeX and
// Plate land in the entry bundle.
import 'katex/dist/katex.min.css'

// GFM-core kits from the shadcn registry. BasicNodesKit already includes BasicMarksKit (importing
// both double-registers the marks), and ListKit already composes IndentKit.
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
import { MarkdownImageKit } from './markdown-image-node'
import { underlineMarkdownRules } from './markdown-underline-rules'

/** GFM-core plugin set shared by the live editor and the round-trip tests, so both serialize
 *  identically. The block affordances (drag handle, `/` menu, toolbars) are UI-only and never
 *  change how a node round-trips. */
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
      remarkPlugins: [remarkGfm, remarkMath],
      // Pin `*italic*` and `- item` (the library defaults are `_` and `*`) so a file the
      // user typed comes back byte-for-byte instead of re-punctuated.
      remarkStringifyOptions: { emphasis: '*', bullet: '-' },
      // Overrides only `blockquote`/`callout` (GitHub alerts), `html` (byte-exact raw HTML),
      // and `underline` serialize (the default throws); Plate merges `rules` per key.
      rules: { ...calloutMarkdownRules, ...htmlMarkdownRules, ...underlineMarkdownRules },
    },
  }),
]
