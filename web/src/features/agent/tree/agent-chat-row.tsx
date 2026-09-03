import { memo } from 'react'
import { ArrowElbowDownRight } from '@phosphor-icons/react'
import { useStore } from 'zustand'
import { cn } from '@/lib/utils'
import {
  ROW_BASE,
  ROW_ACTIVE,
  ROW_GLYPH_BOX,
  ROW_INACTIVE,
  ROW_INDENT_STEP,
  ROW_INDENT_TRANSITION,
  ROW_NEST_TARGET,
  ROW_SUB_ACTION_HOVER,
} from '@/components/layout/workspace-row-base'
import { RowDisclosureButton } from '@/components/layout/row-disclosure-button'
import { FoldAwayButton } from '@/components/layout/fold-away-button'
import { WorkspaceInlineInput } from '@/components/layout/workspace-inline-input'
import { splitHighlight } from '@/features/agent/tree/lib/chat-search'
import { chatRowProps, type ChatDragSubject } from '@/features/agent/tree/lib/chat-drop'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { AgentChatGlyph } from '@/features/agent/shared/agent-chat-glyph'

interface AgentChatRowProps {
  wsId: string
  chatId: string
  title: string
  providerIcon: string
  /** This chat is ON SCREEN — the active tab of a pane the layout renders. */
  active: boolean
  renaming: boolean
  /** This row is in flight — the source stays put and fades. */
  dragging: boolean
  /** A drop would land INSIDE this chat, making the dragged row a thread of it. */
  nesting: boolean
  /** Part of the ⌘-click multiselection a drag would carry. */
  selected: boolean
  /** How deep in the chat tree this row sits. One ROW_INDENT_STEP per level. */
  depth: number
  /** The container this row sits in — published for the drop hit test. */
  parentId: string
  /** This row's ancestor chain, `/a/b/`. The drop policy's own-descendant test. */
  path: string
  expanded: boolean
  hasChildren: boolean
  /** Folded, and still holding a chat the user is looking at. */
  holding: boolean
  /** Kept on screen only as an ancestor of a search hit — drawn dimmed. */
  ctx: boolean
  /** The active query, for marking the matched substring. '' when not filtering. */
  query: string
  /** `meta` is ⌘/Ctrl — a selection gesture, never a request to open the chat. */
  onSelect: (chatId: string, meta: boolean) => void
  onToggle: (chatId: string) => void
  onFoldAway: (chatId: string) => void
  onStartRename: (chatId: string) => void
  onConfirmRename: (chatId: string, title: string) => void
  onCancelRename: () => void
  /**
   * The trailing ↳: start a chat INSIDE this chat — a thread.
   *
   * The same slot and the same signature as the folder row's control, because
   * both mean "make something under this row". The MARK differs, and that is the
   * point: "+" means "add another one of these", which is what a new chat in a
   * folder is, and is not what a thread is. A thread does not sit alongside its
   * parent — it hangs off it and reads it — so the control wears the
   * reply-in-thread elbow instead.
   */
  onNewThread: (parentId: string) => void
  /**
   * Arm a drag.
   *
   * The row builds its own SUBJECT — the same three facts it already publishes
   * as drop attributes — rather than taking a `(id, e)` callback the panel would
   * have to close over per row. That closure was a new function identity on
   * every panel render, which defeated this component's memo entirely: opening
   * one chat re-rendered every row in the list.
   */
  onPointerDownDrag: (subject: ChatDragSubject, e: React.PointerEvent) => void
}

/**
 * One chat in the sidebar tree — a visual sibling of the workspace tree's rows,
 * built from the same tokens so the two columns cannot drift apart.
 *
 * A chat may hold other chats. A child is a THREAD: it hangs off this row and
 * reads this chat's turns. There is no badge and no count, because the INDENT is
 * the entire statement — the same thing it means one panel over. The trailing ↳
 * is how one is made.
 *
 * Memoised, and it reads its OWN turn state straight from the store rather than
 * taking a `working` prop. That combination is load-bearing for a long list: a
 * `turn_started`/`turn_stopped` frame is the hottest thing on the chat feed, and
 * it must re-render ONLY the row it concerns. Every other prop here is a
 * primitive or a stable callback, so a panel re-render for an unrelated reason
 * skips every untouched row.
 */
function AgentChatRowComponent({
  wsId,
  chatId,
  title,
  providerIcon,
  active,
  renaming,
  dragging,
  nesting,
  selected,
  depth,
  parentId,
  path,
  expanded,
  hasChildren,
  holding,
  ctx,
  query,
  onSelect,
  onToggle,
  onFoldAway,
  onStartRename,
  onConfirmRename,
  onCancelRename,
  onNewThread,
  onPointerDownDrag,
}: AgentChatRowProps) {
  // Own turn state, self-subscribed: a primitive selector, so this row re-renders
  // only when THIS chat's spinner flips — never when a sibling's does.
  const working = useStore(
    getOrCreateWorkspaceStore(wsId),
    (s) => s.agentChats.working[chatId] ?? false,
  )

  const { before, hit, after } = splitHighlight(title, query)

  return (
    // The indent box carries margin only — NEVER vertical box. The virtualizer
    // positions rows at index * AGENT_CHAT_ROW_HEIGHT, so any height this wrapper
    // added would desynchronise the drop geometry from the paint.
    <div className={ROW_INDENT_TRANSITION} style={{ marginInlineStart: depth * ROW_INDENT_STEP }}>
      <div
        // react-doctor-disable-next-line prefer-tag-over-role -- can't be a real <button>: while `renaming` this renders a nested <input>, and it carries its own disclosure/fold-away buttons. role="treeitem" is what the sidebar's rows use and what the shape actually is.
        role="treeitem"
        tabIndex={0}
        aria-expanded={hasChildren ? expanded : undefined}
        aria-selected={selected}
        {...chatRowProps({ kind: 'chat', id: chatId, parentId, path, expanded, hasChildren })}
        className={cn(
          ROW_BASE,
          // `group` or the hover-only trailing controls can never come back.
          'group',
          nesting ? ROW_NEST_TARGET : active ? ROW_ACTIVE : ROW_INACTIVE,
          // The source of a drag stays where it is and fades; what is IN THE AIR
          // is the clone the ghost carries.
          dragging && 'opacity-40',
          // Search scaffolding: on screen so a hit never loses its parent chat,
          // but not itself a result.
          ctx && 'opacity-45',
        )}
        onPointerDown={
          renaming ? undefined : (e) => onPointerDownDrag({ kind: 'chat', id: chatId, parentId }, e)
        }
        onClick={renaming ? undefined : (e) => onSelect(chatId, e.metaKey || e.ctrlKey)}
        onDoubleClick={renaming ? undefined : () => onStartRename(chatId)}
        onKeyDown={
          renaming
            ? undefined
            : (e) => {
                if (e.target !== e.currentTarget) return
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault()
                  onSelect(chatId, false)
                }
              }
        }
      >
        {/* Provider icon → flip-dot spinner while working. Neither bakes in a
            colour; both inherit currentColor from the row's own token. */}
        <span className={cn(ROW_GLYPH_BOX, 'relative text-foreground')}>
          <AgentChatGlyph providerIcon={providerIcon} working={working} />
          {/* The three holding dots, on the glyph of a row that is folded over
              something you are looking at — the same mark the folder row draws
              inside its own shape. Under the glyph rather than inside it,
              because a provider's mark is artwork we do not get to edit. */}
          {holding && (
            <span
              data-holding-rows=""
              aria-hidden="true"
              className="pointer-events-none absolute -bottom-0.5 flex gap-px"
            >
              <span className="size-px rounded-full bg-current" />
              <span className="size-px rounded-full bg-current" />
              <span className="size-px rounded-full bg-current" />
            </span>
          )}
        </span>

        {renaming ? (
          <WorkspaceInlineInput
            defaultValue={title}
            placeholder="chat title"
            kind="prose"
            onConfirm={(value) => onConfirmRename(chatId, value)}
            onCancel={onCancelRename}
          />
        ) : (
          <span className="min-w-0 flex-1 truncate">
            {hit ? (
              <>
                {before}
                <mark className="rounded-[3px] bg-sidebar-drop-nest px-px text-inherit">{hit}</mark>
                {after}
              </>
            ) : (
              title
            )}
          </span>
        )}

        {holding && <FoldAwayButton label={title} onFold={() => onFoldAway(chatId)} />}

        {/* On a kept row too. The new thread is OPENED, which is what puts it on
            screen — so it is hoisted out beside its parent under the same holder
            rather than disappearing into the folded row. */}
        <button
          type="button"
          className={ROW_SUB_ACTION_HOVER}
          aria-label={`New thread in ${title}`}
          onClick={(e) => {
            e.stopPropagation()
            onNewThread(chatId)
          }}
          // The row itself arms the drag on pointerdown; without this, pressing
          // the control grabs the row instead of pressing the button.
          onPointerDown={(e) => e.stopPropagation()}
        >
          {/* The reply-in-thread elbow, ↳ — the mark every mail and chat client
              uses for "reply under this". NOT a "+", which says "one more of
              these" and is what a new chat in a FOLDER genuinely is; a thread is
              not another chat alongside this one, it is a conversation that hangs
              off it and reads it live.

              NOT a git-branch or fork glyph either, deliberately: a thread copies
              nothing and takes no snapshot, and a branch mark would promise
              exactly the semantics this design removed.

              ELBOW rather than the softer ArrowBendDownRight: the tree column is
              right angles all the way down (the indent step, the disclosure
              chevron, the fold-away bar), and the bend's curve reads as
              "forward/redirect" beside them. `bold` because the trailing controls
              in this slot sit at ~1–1.5px of stroke in a 12px box — Phosphor's
              regular weight is 0.75px there and would be the faintest mark in the
              row. Phosphor, not Lucide: this column is Phosphor (the duotone
              folder, the provider marks); mixing sets in one column is drift that
              workspace-row-base.ts records having fixed once already. */}
          <ArrowElbowDownRight
            data-thread-glyph="true"
            aria-hidden="true"
            className="size-3"
            weight="bold"
          />
        </button>

        {hasChildren && (
          <RowDisclosureButton expanded={expanded} onToggle={() => onToggle(chatId)} />
        )}
      </div>
    </div>
  )
}

export const AgentChatRow = memo(AgentChatRowComponent)
