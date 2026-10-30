import type { Chat, Workspace } from '@/lib/store/sidebar'
import type { SidebarTreeNode } from '@/components/layout/workspace-tree-utils'

/**
 * The chat that owns a repo/project HOME workspace's worktree, straight off
 * `defaultOwningChatId` — the direct field `rows-from-repo.ts`/`toSidebarRepo`
 * lift from `WorkspaceDTO.owningChatId` for exactly this row, since the home
 * workspace is never a `Workspace` row (it lives on `repo.defaultWorkspaceId`,
 * outside `repo.workspaces` entirely) for {@link resolveOwnerChats} to read
 * directly the way it does for every other row.
 *
 * `defaultOwningChatId` is absent only for a caller with no `Repo` to lift it
 * from at all (`rows-from-home.ts`'s project home, which has no `Repository`
 * either) or a frame that predates the field — both fall back to the same
 * `ownsWorktree` signal {@link resolveOwnerChats} uses for every other row's
 * degrade case. Never a `Chat.type` check: Task 9 stopped minting/retyping a
 * `'branch'`-typed chat for this row, so nothing here may depend on it.
 * Never throws: an unresolved owner leaves the row under its own raw id.
 */
export function resolveHomeOwnerId(
  homeId: string,
  defaultOwningChatId: string | undefined,
  chats: readonly Chat[],
): string {
  if (defaultOwningChatId) return defaultOwningChatId
  const owner = chats.find((c) => c.ownsWorktree && c.workspaceId === homeId)
  return owner?.id ?? homeId
}

/**
 * Every workspace's owning chat, keyed by workspace id — CONFIRMED pairs
 * only, straight off the direct field (2026-09-08 sidebar-placement-
 * unification Task 7/9): every workspace mints its owning chat chat-first,
 * atomically, at creation (`MintOwningChat`/`AttachOwningWorkspace`), and the
 * daemon resolves `Workspace.owningChatId` off it on every `WorkspaceDTO` it
 * sends (`domain.ResolveOwningChat`). No boot backfill races this any more,
 * and the chat is never retyped to mark it (`ChatType` narrows to
 * `'chat'|'workflow'` — see that type's own doc), so this never compares
 * `Chat.type`.
 *
 * "Confirmed" means the WORKSPACE side is present — this is
 * {@link foldWorkspaceOwners}'s whole input, and it deliberately excludes a
 * chat's own unconfirmed `ownsWorktree` claim (see
 * {@link resolveOwnerOfChat}): folding requires a real workspace NODE to fold
 * onto, and a workspace this repo has not seeded has none. A chat whose
 * workspace is merely absent from `workspaces` (deleted, or its own
 * `WorkspaceDTO` has not landed yet) must NOT be treated as foldable, or
 * `foldWorkspaceOwners` strips it from the tree with nowhere to put it back —
 * the exact regression `walkTreeIntoRows`'s own chat branch exists to render
 * instead (see `removal-countdown-rows.test.ts`'s "still draws a branch row
 * off the chat alone").
 *
 * The owner need NOT be in `chats` yet: the DTO is authoritative, and every
 * action path (`workspaceIdOfBranchRow`, `handleCreate`, `forkHasLanded`)
 * already addresses the row by `ws.owningChatId` unconditionally. Folding on
 * the same rule keeps the rendered id and the placement id one id — the
 * daemon mints the owner on the first GET /workspaces, and its chat row can
 * land a structural frame later than the workspace row does.
 */
export function resolveOwnerChats(
  workspaces: readonly Workspace[],
  _chats: readonly Chat[],
): Map<string, string> {
  const ownerChats = new Map<string, string>()
  for (const ws of workspaces) {
    if (ws.owningChatId) ownerChats.set(ws.id, ws.owningChatId)
  }
  return ownerChats
}

/**
 * Chat id -> the workspace it owns, present or not — the WIDER set
 * {@link resolveOwnerChats} deliberately excludes. The authority
 * `walkTreeIntoRows` draws a `branch` row off even when that workspace's own
 * `Workspace` record has not arrived yet (a fresh fork) or is hidden
 * mid-delete: a chat this map names IS a workspace row, full stop, and only
 * its DECORATION (branch name, diff counts, lock) depends on the missing
 * half.
 *
 * Starts from {@link resolveOwnerChats}'s CONFIRMED pairs (reversed), then
 * adds `Chat.ownsWorktree` + `Chat.workspaceId` for whatever workspace that
 * left unconfirmed — delivered atomically WITH the chat, on a separately
 * streamed channel (`crowbar_chats` reseeds on its own folder-signal bump,
 * `crowbar_workspaces` on its own entity stream), so a row still renders as a
 * workspace even when its `WorkspaceDTO` is the half running late. A chat's
 * `workspaceId` alone can NOT stand in for `ownsWorktree`: a thread carries
 * its parent's, so it names a worktree it does not own.
 */
export function resolveOwnerOfChat(
  ownerChats: ReadonlyMap<string, string>,
  chats: readonly Chat[],
): Map<string, string> {
  const ownerOfChat = new Map<string, string>()
  for (const [wsId, chatId] of ownerChats) ownerOfChat.set(chatId, wsId)
  for (const chat of chats) {
    if (!chat.ownsWorktree || !chat.workspaceId || ownerChats.has(chat.workspaceId)) continue
    ownerOfChat.set(chat.id, chat.workspaceId)
  }
  return ownerOfChat
}

/**
 * Fold every workspace-owning chat into the ONE row its workspace already
 * renders as, instead of the two-row split `buildSidebarTree` produces on its
 * own (product rule 6: "a chat with a workspace" is ONE row, not two).
 *
 * Runs on the tree `buildSidebarTree` already built from UNFILTERED
 * workspaces and chats — never on its inputs. That matters: a workspace's own
 * `folderId`/`parentId` lineage is the one edge drag-and-drop actually writes
 * (`WorkspacePlacementWrite`), and `buildSidebarTree`'s folder-anchor
 * compatibility walk, cycle guard and sibling sort already resolve it
 * correctly for a NESTED fork — an owning chat's own `parentId` is used only
 * to find what hangs off it (a real thread), never to decide where the
 * folded row itself renders.
 *
 * Two passes:
 *   1. `stripOwners` removes every owning-chat node whose workspace is ALSO
 *      in this tree, from wherever it sits, and records it (real children
 *      included) by id. Pass 2 always re-attaches its children onto the
 *      workspace it owns, so nothing is spliced back into its old spot.
 *   2. `mergeWorkspaces` relabels every workspace node with a resolved owner
 *      to that owner's id, appending the owner's own (stripped) children
 *      onto its own (none yet when the owner's chat row has not seeded). A
 *      workspace with no owner at all is left completely untouched, exactly
 *      as `buildSidebarTree` built it: still a `branch` row (see
 *      `walkTreeIntoRows`'s workspace branch), just missing the
 *      title/decoration only the chat half carries.
 */
export function foldWorkspaceOwners(
  roots: SidebarTreeNode[],
  ownerChats: ReadonlyMap<string, string>,
): SidebarTreeNode[] {
  const foldableOwnerIds = new Set(ownerChats.values())
  const detached = new Map<string, SidebarTreeNode>()

  const stripOwners = (nodes: SidebarTreeNode[]): SidebarTreeNode[] => {
    const kept: SidebarTreeNode[] = []
    for (const node of nodes) {
      const children = stripOwners(node.children)
      if (node.kind === 'chat' && foldableOwnerIds.has(node.id)) {
        detached.set(node.id, { ...node, children })
        continue
      }
      kept.push({ ...node, children })
    }
    return kept
  }

  const mergeWorkspaces = (nodes: SidebarTreeNode[]): SidebarTreeNode[] =>
    nodes.map((node) => {
      const children = mergeWorkspaces(node.children)
      if (node.kind === 'workspace') {
        const ownerId = ownerChats.get(node.id)
        if (ownerId) {
          const owner = detached.get(ownerId)
          const ownerChildren = owner ? mergeWorkspaces(owner.children) : []
          return { ...node, id: ownerId, children: [...children, ...ownerChildren] }
        }
      }
      return { ...node, children }
    })

  return mergeWorkspaces(stripOwners(roots))
}
