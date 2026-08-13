import { memo } from 'react'
import { Folder, FolderOpen } from '@phosphor-icons/react'
import { cn } from '@/lib/utils'
import {
  ADD_GLYPH_PATH,
  ROW_BASE,
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
import { splitHighlight } from '@/features/agent/lib/chat-search'
import { chatRowProps, type ChatDragSubject } from '@/features/agent/lib/chat-drop'

interface AgentChatFolderRowProps {
  id: string
  name: string
  depth: number
  /** The container this row sits in — published for the drop hit test. */
  parentId: string
  /** This row's ancestor chain, `/a/b/`. The drop policy's own-descendant test. */
  path: string
  expanded: boolean
  hasChildren: boolean
  /** Folded, and still holding a chat the user is looking at. */
  holding: boolean
  renaming: boolean
  dragging: boolean
  /** The row a drop would land INSIDE. */
  nesting: boolean
  /** Part of the ⌘-click multiselection a drag would carry. */
  selected: boolean
  ctx: boolean
  query: string
  /**
   * Collect this row into the ⌘-click multiselection, or drop it out again.
   *
   * Called from the ⌘/Ctrl branch alone: a plain click folds the row, so a
   * "was this the selection gesture" flag would name a call that never happens.
   */
  onSelect: (id: string) => void
  onToggle: (id: string) => void
  onFoldAway: (id: string) => void
  onStartRename: (id: string) => void
  onConfirmRename: (id: string, name: string) => void
  onCancelRename: () => void
  /**
   * The trailing "+": start a chat inside this folder.
   *
   * A chat, not a folder. "+" on a row means "add a child of the tree's primary
   * kind" everywhere in the sidebar, and the primary kind here is a
   * conversation — folders are made around a selection, from the menu.
   */
  onAddChat: (parentId: string) => void
  /** Arm a drag. The row builds its own subject — see AgentChatRow for why. */
  onPointerDownDrag: (subject: ChatDragSubject, e: React.PointerEvent) => void
}

/**
 * A grouping folder in the Chats tree — the workspace tree's folder row, drawn
 * against chats instead of branches.
 *
 * Same contract as every other row in the sidebar: `role="treeitem"` (never
 * `role="button"`, which takes PRESENTATIONAL children and would erase this
 * row's own `+` and chevron), `aria-expanded` reflecting its own state, chrome
 * from ROW_BASE so it cannot drift from the chat rows it interleaves with.
 *
 * A folder has no destination, so the row body toggles it.
 */
function AgentChatFolderRowComponent({
  id,
  name,
  depth,
  parentId,
  path,
  expanded,
  hasChildren,
  holding,
  renaming,
  dragging,
  nesting,
  selected,
  ctx,
  query,
  onSelect,
  onToggle,
  onFoldAway,
  onStartRename,
  onConfirmRename,
  onCancelRename,
  onAddChat,
  onPointerDownDrag,
}: AgentChatFolderRowProps) {
  const { before, hit, after } = splitHighlight(name, query)

  return (
    <div className={ROW_INDENT_TRANSITION} style={{ marginInlineStart: depth * ROW_INDENT_STEP }}>
      <div
        // react-doctor-disable-next-line prefer-tag-over-role -- a <button> may not contain the rename <input> this row swaps in, nor its own nested + and chevron buttons.
        role="treeitem"
        tabIndex={-1}
        aria-expanded={expanded}
        aria-selected={selected}
        {...chatRowProps({ kind: 'chatFolder', id, parentId, path, expanded, hasChildren })}
        className={cn(
          ROW_BASE,
          'group',
          nesting ? ROW_NEST_TARGET : ROW_INACTIVE,
          dragging && 'opacity-40',
          ctx && 'opacity-45',
        )}
        onClick={(e) => {
          if (renaming) return
          // ⌘/Ctrl is the selection gesture everywhere in the sidebar, and it
          // must not also fold the row it is collecting.
          if (e.metaKey || e.ctrlKey) onSelect(id)
          else onToggle(id)
        }}
        // Guarded on `renaming` like every other handler on this row. The editor
        // is a nested <input> that does not stop `dblclick`, so double-clicking
        // a WORD inside it bubbled here and re-opened the rename that was
        // already open — which throws the caret back to the start of the field
        // in the middle of typing.
        onDoubleClick={
          renaming
            ? undefined
            : (e) => {
                e.stopPropagation()
                onStartRename(id)
              }
        }
        onKeyDown={(e) => {
          if (e.target !== e.currentTarget) return
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            onToggle(id)
          }
        }}
        onPointerDown={
          renaming ? undefined : (e) => onPointerDownDrag({ kind: 'chatFolder', id, parentId }, e)
        }
      >
        {/* Open and closed are two glyphs, SWAPPED, not one tweening between
            them: at 16px an open folder is a few pixels of shear from a closed
            one, so a tween spends 200ms saying what the swap says instantly.
            Duotone rather than fill, because the holding dots sit INSIDE the
            glyph and a solid folder leaves them nowhere to be. Their
            coordinates are Phosphor's 256-unit viewBox, not Lucide's 24. */}
        <span className={ROW_GLYPH_BOX}>
          {expanded ? (
            <FolderOpen aria-hidden="true" className="size-4" weight="duotone" />
          ) : (
            <Folder aria-hidden="true" className="size-4" weight="duotone">
              {holding && (
                <g data-holding-rows="" fill="currentColor" stroke="none">
                  <circle cx="85" cy="160" r="12" />
                  <circle cx="128" cy="160" r="12" />
                  <circle cx="171" cy="160" r="12" />
                </g>
              )}
            </Folder>
          )}
        </span>

        {renaming ? (
          <WorkspaceInlineInput
            defaultValue={name}
            placeholder="folder-name"
            // Free text someone typed, not a git ref, so the editor reads in the
            // same face as the label it replaced.
            kind="prose"
            onConfirm={(value) => onConfirmRename(id, value)}
            onCancel={onCancelRename}
          />
        ) : (
          // Sans and 600, against the chat rows' 500. The change of face is the
          // cheapest signal that this row is a container, not a conversation.
          <span className="min-w-0 flex-1 truncate text-left font-sans font-semibold tracking-[0.005em]">
            {hit ? (
              <>
                {before}
                <mark className="rounded-[3px] bg-sidebar-drop-nest px-px text-inherit">{hit}</mark>
                {after}
              </>
            ) : (
              name
            )}
          </span>
        )}

        {holding && <FoldAwayButton label={name} onFold={() => onFoldAway(id)} />}

        <button
          type="button"
          className={ROW_SUB_ACTION_HOVER}
          aria-label={`New chat in ${name}`}
          onClick={(e) => {
            e.stopPropagation()
            onAddChat(id)
          }}
          onPointerDown={(e) => e.stopPropagation()}
        >
          <svg
            aria-hidden="true"
            className="size-3"
            viewBox="0 0 16 16"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
          >
            <path d={ADD_GLYPH_PATH} />
          </svg>
        </button>

        {hasChildren && <RowDisclosureButton expanded={expanded} onToggle={() => onToggle(id)} />}
      </div>
    </div>
  )
}

export const AgentChatFolderRow = memo(AgentChatFolderRowComponent)
