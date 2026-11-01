import { MarkdownPlugin } from '@platejs/markdown'
import { CodeBlockRules } from '@platejs/code-block'
import { CodeBlockPlugin, CodeLinePlugin } from '@platejs/code-block/react'
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
import { ChatFloatingToolbarKit } from '@/features/agent/composer/plate/chat-floating-toolbar-kit'
import { HtmlKit } from '@/features/editor/markdown/plate/html-node'
import { MarkdownImageKit } from '@/features/editor/markdown/plate/markdown-image-node'
import { calloutMarkdownRules } from '@/features/editor/markdown/plate/markdown-callout-rules'
import { htmlMarkdownRules } from '@/features/editor/markdown/plate/markdown-html-rules'
import { underlineMarkdownRules } from '@/features/editor/markdown/plate/markdown-underline-rules'
import { ChatFreshTextPlugin } from '@/features/agent/transcript/plate/chat-fresh-text-plugin'
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

// One set for prompts and answers so they can't drift. @platejs/markdown drops any
// unregistered node, so every node type is registered; SlashKit is left out because
// `/` opens Crowbar's skill picker, MathKit for its ~280 KB.

// Two independent `.configure()` calls: chaining `.withComponent()` onto one shared
// instance resolves the node config differently at render time.
const CHAT_CODE_BLOCK_CONFIG = {
  inputRules: [CodeBlockRules.markdown({ on: 'match' })],
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
  // 16px, not the shared 24px: Plate writes it as an inline style no stylesheet reaches.
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
  // After BasicNodesKit so it replaces its paragraph (a later entry with the same key
  // wins), making every paragraph a drop target.
  ParagraphPlugin.withComponent(ChatParagraphElement),
  ...ChatFloatingToolbarKit,
  // Inert outside streaming: only streaming-value-patch.ts sets its mark.
  ChatFreshTextPlugin,
  MarkdownPlugin.configure({
    options: {
      remarkPlugins: [remarkGfm],
      // The same punctuation the file and comment editors pin.
      remarkStringifyOptions: { emphasis: '*', bullet: '-' },
      rules: { ...calloutMarkdownRules, ...htmlMarkdownRules, ...underlineMarkdownRules },
    },
  }),
]

const STATIC_NODE_OVERRIDES: Record<string, (typeof chatComposerPlugins)[number]> = {
  [LinkPlugin.key]: ChatLinkKitStatic[0],
  [CalloutPlugin.key]: CalloutKitStatic[0],
  [CodeBlockPlugin.key]: ChatCodeBlockPluginStatic,
  [ParagraphPlugin.key]: ParagraphPlugin.withComponent(ParagraphElement),
  [MarkdownImageKit[0].key]: MarkdownImageKit[0].configure({
    node: { component: ChatMarkdownImageElementStatic },
  }),
}

// `PlateStatic` still renders `afterEditable`, and FloatingToolbar's
// `useEditorId()` crashes without an interactive Plate.
const STATIC_EXCLUDED_KEYS = new Set(ChatFloatingToolbarKit.map((plugin) => plugin.key))

/** Derived, not hand-duplicated, so a plugin added above flows through. */
export const chatComposerPluginsStatic = chatComposerPlugins.reduce<typeof chatComposerPlugins>(
  (plugins, plugin) => {
    if (!STATIC_EXCLUDED_KEYS.has(plugin.key))
      plugins.push(STATIC_NODE_OVERRIDES[plugin.key] ?? plugin)
    return plugins
  },
  [],
)
