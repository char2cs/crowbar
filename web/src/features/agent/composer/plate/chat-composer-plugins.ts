import { MarkdownPlugin } from '@platejs/markdown'
import { CodeBlockRules } from '@platejs/code-block'
import { CodeBlockPlugin, CodeLinePlugin } from '@platejs/code-block/react'
import { DndPlugin } from '@platejs/dnd'
import { NodeIdPlugin } from 'platejs'
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
 * - `BlockMenuKit`, `BlockPlaceholderKit` — drag handles and a "type / for
 *   commands" hint that would be a lie here. Page-editor furniture on a box
 *   that is usually one line.
 *
 * `ChatFloatingToolbarKit` (not the file editor's `FloatingToolbarKit`) IS
 * registered — a selection toolbar earns its place once there's a selection
 * to make, whether that's a one-line prompt or the empty-document surface.
 * It's the comment editor's button set (no inline-equation button — no
 * `MathKit` here either), not the full editor's.
 *
 * Table and code-block nodes use the COMMENT editor's minimal components rather
 * than the file editor's, which pull `createLowlight(all)`, cmdk and a Radix
 * popover for affordances a chat has no use for.
 */
// Shared shape for both variants below — everything but the node component,
// which each `.configure()` call sets directly. `CodeBlockPlugin.configure(...)
// .withComponent(...)` was tried first, chaining onto one shared instance, but
// `withComponent`'s `.extend()`-based merge and `configure`'s own node-config
// handling resolve differently at render time (confirmed by mounting each
// through a real editor, not just inspecting the returned plugin object) — two
// independent `.configure()` calls, the same pattern already proven correct
// for the interactive plugin before this task, is what actually works for
// both.
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

// Same plugin as `MarkdownImageKit`'s, only its node component swapped —
// chat's own reason to override it (unlike code_block's) is the same for
// both interactive and static rendering, so one configured plugin replaces
// the kit's single entry rather than needing an interactive/static pair.
const ChatMarkdownImageKit = [
  MarkdownImageKit[0].configure({ node: { component: ChatMarkdownImageElement } }),
]

export const chatComposerPlugins = [
  ...BasicNodesKit,
  ...ListKit,
  // The canvas indents a list 16px a level; the shared kit's step is 24px, and
  // Plate writes it as an INLINE style — no stylesheet can reach it, so it is
  // set where it is computed. Re-configuring here rather than in `IndentKit`
  // keeps the file editor's own rhythm out of it.
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
  // `@platejs/dnd`'s bridge plugin — needed for `useAttachmentDraggable`
  // (attachment-drag-handle.tsx) to do anything at all: `useDraggable`'s own
  // implementation short-circuits to `{}` (no drag, no drop line, no
  // `<DndProvider>` requirement) whenever `editor.plugins.dnd` is unset. This
  // is the plugin the upstream Plate registry calls `dnd-kit.tsx` — never
  // added to this app before (see attachment-drag-handle.tsx's own note);
  // registered bare (no `enableScroller`) since chat has no long vertical
  // document to auto-scroll while dragging, the way a full page editor does.
  DndPlugin,
  // `@platejs/dnd`'s own hover/drop-target resolution keys everything off
  // `element.id` — `getHoverDirection` explicitly bails when the candidate
  // you're hovering shares the DRAGGED item's id, which is EVERY candidate
  // when nothing ever assigns one: every block's `.id` is `undefined`, and
  // `undefined === undefined`. Confirmed directly (no plugin under any key
  // in `editor.plugins` assigns one without this — this app never actually
  // had one, an incorrect assumption from earlier in this feature's build).
  // Without it, a drop target could never be distinguished from the thing
  // being dragged, live or in a test — attachment-to-attachment reordering
  // "worked" only in the sense that a stuck `dropTarget` state (see the
  // `drag.end` fix, attachment-drag-handle.tsx) happened to render a line
  // SOMEWHERE, not because hover ever legitimately resolved a target.
  NodeIdPlugin,
  // A plain paragraph is otherwise never a valid drop target at all — only
  // an attachment block registers with `@platejs/dnd`, so an attachment
  // could only ever swap places with ANOTHER attachment. Registered after
  // `...BasicNodesKit` (whose own `ParagraphPlugin.withComponent(Paragraph
  // Element)` this replaces — a later entry with the same `.key` wins,
  // confirmed empirically rather than assumed) so an attachment can be
  // dropped anywhere a paragraph can, not just next to another attachment.
  ParagraphPlugin.withComponent(ChatParagraphElement),
  ...ChatFloatingToolbarKit,
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
      rules: { ...calloutMarkdownRules, ...htmlMarkdownRules, ...underlineMarkdownRules },
    },
  }),
]

const STATIC_NODE_OVERRIDES: Record<string, (typeof chatComposerPlugins)[number]> = {
  [LinkPlugin.key]: ChatLinkKitStatic[0],
  [CalloutPlugin.key]: CalloutKitStatic[0],
  // Same plugin, only its node component swapped: `ChatCodeBlockElementStatic`
  // never renders the drag handle (or calls `@platejs/dnd`'s `useDraggable`)
  // that `ChatCodeBlockElement` does — a settled message is read, not
  // reordered, and has no `<DndProvider>` ancestor to call it against.
  [CodeBlockPlugin.key]: ChatCodeBlockPluginStatic,
  // Back to the plain, shared `ParagraphElement` — a settled message is read,
  // not reordered, and has nothing to drop an attachment ONTO it for.
  [ParagraphPlugin.key]: ParagraphPlugin.withComponent(ParagraphElement),
  // Same reasoning as `CodeBlockPlugin` above — `ChatMarkdownImageElementStatic`
  // keeps the height cap, drops the drag handle.
  [MarkdownImageKit[0].key]: MarkdownImageKit[0].configure({
    node: { component: ChatMarkdownImageElementStatic },
  }),
}

// `PlateStatic` still renders `render.afterEditable` (see @platejs/core's
// static build) even though there's no selection to make one for — a settled
// message would otherwise crash on `FloatingToolbar`'s `useEditorId()`, which
// requires an interactive `Plate`/`PlateController` that static rendering
// never provides. Dropped here, not swapped, because there's no static
// equivalent of a selection toolbar.
//
// `DndPlugin` itself is dropped too — a settled message is read, not
// reordered, and neither static node component
// (`ChatCodeBlockElementStatic`/`ChatLinkElementStatic`'s file card) ever
// calls `useAttachmentDraggable`, so there is nothing here that would read
// `editor.plugins.dnd` in the first place.
const STATIC_EXCLUDED_KEYS = new Set([
  ...ChatFloatingToolbarKit.map((plugin) => plugin.key),
  DndPlugin.key,
])

/**
 * `chatComposerPluginsStatic`, derived — not hand-duplicated. A plugin added above
 * flows through automatically; only registered exceptions (Link's toolbar,
 * Callout's icon picker — both need an interactive editor, neither is a
 * content difference, see callout-content.tsx/link-kit.tsx) get swapped, and
 * `ChatFloatingToolbarKit` gets dropped entirely (see STATIC_EXCLUDED_KEYS).
 */
export const chatComposerPluginsStatic = chatComposerPlugins.reduce<typeof chatComposerPlugins>(
  (plugins, plugin) => {
    if (!STATIC_EXCLUDED_KEYS.has(plugin.key))
      plugins.push(STATIC_NODE_OVERRIDES[plugin.key] ?? plugin)
    return plugins
  },
  [],
)
