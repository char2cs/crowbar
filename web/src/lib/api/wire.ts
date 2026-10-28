import type { ChatDTO, ChatType, ChatWorktreeDTO, FolderDTO, WorkspaceDTO } from '@/lib/types'

/**
 * The wire shape of one row GET .../chats/folders returns: a folder-typed
 * domain.Chat rendered through dto.AgentChatDTO (Task 34 — the dedicated
 * `/folders` resource was deleted; folders are Chat rows now). Neither repoId
 * nor projectId travel on it — the URL is the only place either is known —
 * and its display text is `title`, not `name`.
 */
export interface ChatsFolderWireDTO {
  id: string
  /** The row's kind: a folder, or — among a write's `shifted` siblings — a
   *  locked branch reported by its workspace id. Absent means folder. */
  type?: string
  parentId: string
  title: string
  order: number
  /** ISO creation time — the daemon's `order` tiebreak (tree.compareNodes). */
  createdAt?: string
}

/** `ChatsFolderWireDTO` -> the sidebar's own `FolderDTO`, filling in the
 *  repo/project scope the wire row doesn't carry. Shared by `fetchFolders`
 *  and `sidebar-placement.ts`'s write verbs, which read the same rows back
 *  off their own mutation responses. */
export function folderDTOFromWire(
  row: ChatsFolderWireDTO,
  projectId: string,
  repoId: string,
): FolderDTO {
  return {
    id: row.id,
    repoId,
    projectId,
    parentId: row.parentId,
    name: row.title,
    order: row.order,
    createdAt: row.createdAt,
  }
}

/**
 * The wire shape of one row GET .../repos/:r/chats returns — the conversation
 * half of the same `dto.AgentChatDTO` `.../chats/folders` serves the folder half
 * of. Only the fields a TREE row needs are declared: every runner-derived field
 * on that DTO (liveRunnerId, terminalSessionId, activeProviderId, telemetry) is
 * about a PROCESS, and the tree answers "does this exist", not "what is up right
 * now" (design spec §5.7 — the live half is Recents' question, off the workspace
 * store's own live chat list).
 */
export interface RepoChatWireDTO {
  id: string
  workspaceId: string
  parentId: string
  title: string
  order: number
  /** ISO creation time — the daemon's `order` tiebreak (tree.compareNodes),
   *  so a level nobody has dragged draws in the sequence a drop is counted. */
  createdAt?: string
  /** The row's own kind. Always sent by the daemon (dto.AgentChatDTO.Type is
   *  never omitted — "" is not a real ChatType), so an absent value here only
   *  ever means a frame older than the field. */
  type?: ChatType
  /** The worktree this row HOLDS. Present iff `workspaceId` is non-empty — on
   *  EVERY row carrying that workspace, not just the one that owns it. */
  worktree?: ChatWorktreeDTO
}

/**
 * The one place a `ChatWorktreeDTO` becomes a `WorkspaceDTO`.
 *
 * Both the chat LIST (`workspaceDTOFromChat`) and the chat lifecycle STREAM
 * (`workspaceDTOFromWorktreeFrame`) carry the same nested object, so they map it
 * through here rather than each grounding the optionals their own way — the
 * sidebar merges a live frame over a seeded row field by field
 * (`build-repo-tree.ts`'s `toSidebarWorkspace`), and two mappings that disagree
 * on what an absent `prUrl` means would show up as a field that never clears.
 */
function workspaceDTOFromWorktree(
  worktree: ChatWorktreeDTO,
  workspaceId: string,
  projectId: string,
  repoId: string,
): WorkspaceDTO {
  return {
    id: workspaceId,
    repoId,
    projectId,
    branch: worktree.branch ?? '',
    parentId: worktree.parentId ?? '',
    forkPointSha: worktree.forkPointSha ?? '',
    status: worktree.status ?? 'new',
    working: worktree.working ?? false,
    lastError: worktree.lastError ?? '',
    isDefault: worktree.isDefault ?? false,
    added: worktree.added ?? 0,
    deleted: worktree.deleted ?? 0,
    mergeStrategy: worktree.mergeStrategy ?? '',
    canMergeLocally: worktree.canMergeLocally ?? false,
    mergeConflicts: worktree.mergeConflicts ?? false,
    parentBranch: worktree.parentBranch ?? '',
    prUrl: worktree.prUrl ?? '',
    prTitle: worktree.prTitle ?? '',
    prTargetBranch: worktree.prTargetBranch ?? '',
    localPath: worktree.localPath ?? '',
    heldByPath: worktree.heldByPath ?? '',
    provisioning: worktree.provisioning,
    owningChatId: worktree.owningChatId,
    folderId: worktree.folderId ?? '',
    order: worktree.order ?? 0,
    createdAt: worktree.createdAt,
  }
}

/**
 * One chat row -> the `WorkspaceDTO` for the worktree it OWNS, or null.
 *
 * Null covers all three ways a row is not a worktree's row: a bubble chat holds
 * none at all, a thread carries its parent's `worktree` object but is not the
 * row that owns it (`row.id !== worktree.owningChatId`), and a row with no
 * `workspaceId` has no id to key the workspace by. Mapping a whole chat list
 * through this therefore yields exactly one DTO per worktree, however many rows
 * share it.
 */
export function workspaceDTOFromChat(
  row: RepoChatWireDTO,
  projectId: string,
  repoId: string,
): WorkspaceDTO | null {
  const worktree = row.worktree
  if (!worktree || worktree.owningChatId !== row.id || !row.workspaceId) return null
  return workspaceDTOFromWorktree(worktree, row.workspaceId, projectId, repoId)
}

/** One frame of the chat lifecycle feed (`AgentChatEvent`), as far as the
 *  worktree half is concerned. Every other kind rides the same socket. */
interface WorktreeStateFrame {
  chatId?: string
  workspaceId?: string
  kind?: string
  /**
   * The repo the frame's own row runs in (`AgentChatEvent.RepoID`). On a
   * `worktree_state` frame this is authoritative, never derived: the daemon
   * takes it straight off the workspace (`container.go`'s `pushChatWorktree` —
   * "a worktree-owning row names its own repo outright"). Absent means the
   * worktree has no repo AT ALL — the project-home worktree.
   */
  repoId?: string
  worktree?: ChatWorktreeDTO
}

/**
 * A chat-stream frame -> a `WorkspaceDTO`, or null for every frame that is not
 * this chat's own worktree state IN THIS REPO.
 *
 * The chat sockets carry lifecycle EVENTS, not entity DTOs, and most kinds
 * (`turn_started`, `deleted`, `folder_created`, …) say nothing about a worktree.
 * The owning-row rule is the same one `workspaceDTOFromChat` applies to the
 * list: a thread of the owning chat gets `worktree_state` frames too, and
 * letting one through would write the workspace under the wrong chat's identity.
 *
 * The REPO rule is the second half, and `repoId` here is a caller's SCOPE, not
 * a fact about the frame — every caller passes the repo whose socket/route it
 * subscribed under, and this function stamps it onto the DTO it mints. So it
 * may only mint when the frame agrees that its worktree really is that repo's.
 *
 * A repo-scoped chats socket does NOT only carry that repo's frames: the daemon
 * holds a frame that knows its repo to exactly that repo, but deliberately fans
 * a frame with an EMPTY repo out to every subscriber (`container.go`'s
 * `matchRepoOrUnscoped`) so the live folder feed and root bubbles — rows that
 * genuinely have no repo — are not silently dropped. The project-home worktree
 * has no repo either, so its `worktree_state` reaches every repo's socket; each
 * one used to stamp its OWN repo id onto it and mint the home workspace as that
 * repo's workspace, producing one labelless `branch` row (its branch is '')
 * under every repo header, per project, the instant a home chat took a turn.
 * Live-reported twice. Never re-derive the row away downstream — the row must
 * not be constructible.
 */
export function workspaceDTOFromWorktreeFrame(
  raw: unknown,
  projectId: string,
  repoId: string,
): WorkspaceDTO | null {
  const frame = raw as WorktreeStateFrame | null
  if (!frame || frame.kind !== 'worktree_state') return null
  const worktree = frame.worktree
  if (!worktree || !frame.workspaceId || worktree.owningChatId !== frame.chatId) return null
  if ((frame.repoId ?? '') !== repoId) return null
  return workspaceDTOFromWorktree(worktree, frame.workspaceId, projectId, repoId)
}

/** `RepoChatWireDTO` -> the sidebar's own `ChatDTO`, filling in the repo/project
 *  scope the wire row doesn't carry (the URL is the only place either is known —
 *  the same rule `folderDTOFromWire` follows). */
export function chatDTOFromWire(row: RepoChatWireDTO, projectId: string, repoId: string): ChatDTO {
  return {
    id: row.id,
    repoId,
    projectId,
    type: row.type,
    workspaceId: row.workspaceId ?? '',
    // The SAME predicate `workspaceDTOFromChat` above uses to decide this row
    // is the worktree's row, kept on the chat instead of only being spent
    // deriving a separate `WorkspaceDTO`. `worktree` rides EVERY row holding
    // the workspace (a thread carries its parent's), so `owningChatId` is the
    // only thing that picks the owner out — see ChatDTO.ownsWorktree for why
    // the answer has to travel with the chat rather than be re-joined later.
    ownsWorktree: row.worktree?.owningChatId === row.id,
    parentId: row.parentId,
    title: row.title,
    order: row.order ?? 0,
    createdAt: row.createdAt,
  }
}
