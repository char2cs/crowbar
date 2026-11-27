import { MarkdownPlugin } from '@platejs/markdown'
import { type CodeBlockConfig, CodeBlockRules } from '@platejs/code-block'
import { CodeBlockPlugin, CodeLinePlugin, CodeSyntaxPlugin } from '@platejs/code-block/react'
import remarkGfm from 'remark-gfm'

import { BasicNodesKit } from '@/components/editor/plugins/basic-nodes-kit'
import { CalloutKit } from '@/components/editor/plugins/callout-kit'
import { IndentPlugin } from '@platejs/indent/react'
import { ListKit } from '@/components/editor/plugins/list-kit'
import { ParagraphPlugin } from 'platejs/react'
import { ParagraphElement } from '@/components/ui/paragraph-node'
import {
  ChatLinkKit,
  ChatLinkKitStatic,
} from '@/features/agent/composer/plate/attachments/chat-link-kit'
import { LinkPlugin } from '@platejs/link/react'
import { CalloutKitStatic } from '@/components/editor/plugins/callout-kit-static'
import { CalloutPlugin } from '@platejs/callout/react'
import {
  TableCellHeaderPlugin,
  TableCellPlugin,
  TablePlugin,
  TableRowPlugin,
} from '@platejs/table/react'
import { MarkFloatingToolbarKit } from '@/components/editor/plugins/mark-floating-toolbar-kit'
import { HtmlKit } from '@/features/editor/markdown/plate/html-node'
import { MarkdownImageKit } from '@/features/editor/markdown/plate/markdown-image-node'
import { calloutMarkdownRules } from '@/features/editor/markdown/plate/markdown-callout-rules'
import { htmlMarkdownRules } from '@/features/editor/markdown/plate/markdown-html-rules'
import { underlineMarkdownRules } from '@/features/editor/markdown/plate/markdown-underline-rules'
import { ChatFreshTextPlugin } from '@/features/agent/transcript/plate/chat-fresh-text-plugin'
import { ChatMissingPathPlugin } from '@/features/agent/transcript/plate/chat-missing-path-plugin'
import { shikiLowlight } from '@/components/editor/plugins/shiki-lowlight'
import { CodeSyntaxLeaf } from '@/components/ui/code-block-node'
import { CodeSyntaxLeafStatic } from '@/components/ui/code-syntax-leaf-static'
import {
  CommentCodeLineElement,
  CommentTableCellElement,
  CommentTableCellHeaderElement,
  CommentTableElement,
  CommentTableRowElement,
} from '@/features/editor/markdown/plate/comment/comment-nodes'
import {
  ChatCodeBlockElement,
  ChatCodeBlockElementStatic,
} from '@/features/agent/composer/plate/attachments/chat-code-block-node'
import {
  ChatMarkdownImageElement,
  ChatMarkdownImageElementStatic,
} from '@/features/agent/composer/plate/attachments/chat-markdown-image-node'
import { ChatParagraphElement } from '@/features/agent/composer/plate/attachments/chat-paragraph-node'

const CHAT_CODE_BLOCK_CONFIG = {
  inputRules: [CodeBlockRules.markdown({ on: 'match' })],
  options: { lowlight: shikiLowlight as unknown as CodeBlockConfig['options']['lowlight'] },
  shortcuts: { toggle: { keys: 'mod+alt+8' } },
}

const ChatCodeBlockPlugin = CodeBlockPlugin.configure({
  ...CHAT_CODE_BLOCK_CONFIG,
  node: { component: ChatCodeBlockElement },
})

const ChatCodeBlockPluginStatic = CodeBlockPlugin.configure({
  ...CHAT_CODE_BLOCK_CONFIG,
  node: { component: ChatCodeBlockElementStatic },
})

const ChatMarkdownImageKit = [
  MarkdownImageKit[0].configure({ node: { component: ChatMarkdownImageElement } }),
]

export const chatComposerPlugins = [
  ...BasicNodesKit,
  ...ListKit,
  IndentPlugin.configure({ options: { offset: 16 } }),
  ...ChatLinkKit,
  ...HtmlKit,
  ...CalloutKit,
  ...ChatMarkdownImageKit,
  TablePlugin.withComponent(CommentTableElement),
  TableRowPlugin.withComponent(CommentTableRowElement),
  TableCellPlugin.withComponent(CommentTableCellElement),
  TableCellHeaderPlugin.withComponent(CommentTableCellHeaderElement),
  ChatCodeBlockPlugin,
  CodeLinePlugin.withComponent(CommentCodeLineElement),
  CodeSyntaxPlugin.withComponent(CodeSyntaxLeaf),
  ParagraphPlugin.withComponent(ChatParagraphElement),
  ...MarkFloatingToolbarKit,
  ChatFreshTextPlugin,
  ChatMissingPathPlugin,
  MarkdownPlugin.configure({
    options: {
      remarkPlugins: [remarkGfm],
      remarkStringifyOptions: { emphasis: '*', bullet: '-' },
      rules: { ...calloutMarkdownRules, ...htmlMarkdownRules, ...underlineMarkdownRules },
    },
  }),
]

const STATIC_NODE_OVERRIDES: Record<string, (typeof chatComposerPlugins)[number]> = {
  [LinkPlugin.key]: ChatLinkKitStatic[0],
  [CalloutPlugin.key]: CalloutKitStatic[0],
  [CodeBlockPlugin.key]: ChatCodeBlockPluginStatic,
  [CodeSyntaxPlugin.key]: CodeSyntaxPlugin.withComponent(CodeSyntaxLeafStatic),
  [ParagraphPlugin.key]: ParagraphPlugin.withComponent(ParagraphElement),
  [MarkdownImageKit[0].key]: MarkdownImageKit[0].configure({
    node: { component: ChatMarkdownImageElementStatic },
  }),
}

const STATIC_EXCLUDED_KEYS = new Set(MarkFloatingToolbarKit.map((plugin) => plugin.key))

export const chatComposerPluginsStatic = chatComposerPlugins.reduce<typeof chatComposerPlugins>(
  (plugins, plugin) => {
    if (!STATIC_EXCLUDED_KEYS.has(plugin.key))
      plugins.push(STATIC_NODE_OVERRIDES[plugin.key] ?? plugin)
    return plugins
  },
  [],
)
