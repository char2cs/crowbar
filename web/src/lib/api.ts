import type {
  ChatDTO,
  ChatType,
  ChatWorktreeDTO,
  FolderDTO,
  Project,
  Prerequisites,
  RepoDTO,
  WorkspaceDTO,
} from './types'
import type { PRLink } from '@/lib/import/parent-plan'
import { useChaosStore } from '@/lib/store/chaos'
import { getOwningChatId } from '@/lib/workspace-scope'
import { worktreeVerbBaseForWorkspace } from '@/lib/workspace-scope-url'

const crowbar = (window as unknown as { __CROWBAR__?: { api?: string } }).__CROWBAR__
export const API_BASE: string = crowbar?.api ?? import.meta.env.VITE_API_URL ?? ''

/**
 * Turn a daemon-relative asset path into one the webview can actually load.
 *
 * Only for URLs handed to the BROWSER — an `<img src>`, not an `apiFetch` (which
 * applies API_BASE itself). A DTO's icon URL arrives as a bare `/v0/...` path,
 * and the desktop webview is served from its own origin: on the dev server that
 * path resolves to Vite, in a packaged build to the app bundle. Either way it is
 * not the daemon, so the request 404s and the <img> quietly falls back to the
 * entity's default mark — a broken icon that looks exactly like an icon nobody
 * set.
 *
 * Defined here, beside API_BASE, so the repo avatar and the project icon cannot
 * resolve the same kind of URL two different ways.
 */
export function assetURL(path: string): string {
  return `${API_BASE}${path}`
}

/** Error thrown by apiFetch carrying the HTTP status, so callers can make
 *  status-specific decisions (e.g. a 404 is terminal — never retried). */
export class ApiError extends Error {
  readonly status: number
  /** Stable server recovery category. Most endpoints omit it. */
  readonly code?: string
  constructor(message: string, status: number, code?: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

export function isNotFoundError(err: unknown): boolean {
  return err instanceof ApiError && err.status === 404
}

/** Tunable transient-retry policy for {@link apiFetch}. A `fetch()` *rejection*
 *  (a network/transport error — never an HTTP status) means the request never
 *  reached the daemon; at app launch that is the sidecar's unix socket not yet
 *  accepting connections. Idempotent reads retry across that startup window so a
 *  cold start lands on the workspace instead of crashing or showing an empty
 *  state. Injectable so tests run without real backoff sleeps. */
export interface RetryConfig {
  /** Total attempts including the first (1 = no retry). */
  attempts: number
  /** Exponential backoff base; delay = baseDelayMs * 2^(attempt-1), capped. */
  baseDelayMs: number
  maxDelayMs: number
  sleep?: (ms: number) => Promise<void>
}

// ~5s of bounded backoff (0,100,200,400,800,1000,1000,1000) comfortably covers a
// daemon sidecar's socket-bind window without masking a genuinely-down daemon.
const DEFAULT_RETRY: RetryConfig = { attempts: 8, baseDelayMs: 100, maxDelayMs: 1000 }

const defaultSleep = (ms: number): Promise<void> => new Promise((r) => setTimeout(r, ms))

/** Only idempotent reads auto-retry on transport failure. Mutations have their
 *  own async-202 + subscribe-before-POST resilience and must never be blindly
 *  replayed (at-least-once would double-apply). */
function isIdempotentRead(init?: RequestInit): boolean {
  const method = init?.method?.toUpperCase()
  return method === undefined || method === 'GET'
}

/** Perform a request with the shared chaos headers, transient-transport retry and
 *  error-status handling, returning the raw `Response`.
 *
 *  This is the layer beneath {@link apiFetch}: every v0 route answers the
 *  `{success,data}` envelope EXCEPT `/review/patch`, which answers text/plain
 *  and reports truncation in a response header. That one caller needs the
 *  Response itself — its body is not JSON and its headers carry meaning — but it
 *  needs the retry and the error-envelope decoding just as much, so both live
 *  here rather than being reimplemented beside it. */
export async function apiFetchRaw(
  path: string,
  init?: RequestInit,
  retry: RetryConfig = DEFAULT_RETRY,
): Promise<Response> {
  const { latency, errorRate, scenario, faults } = useChaosStore.getState()
  const chaosHeaders: Record<string, string> = {}
  if (latency > 0) chaosHeaders['X-Crowbar-Latency'] = String(latency)
  if (errorRate > 0) chaosHeaders['X-Crowbar-Error-Rate'] = String(errorRate)

  if (import.meta.env.VITE_USE_MOCK === 'true') {
    chaosHeaders['X-Crowbar-Scenario'] = scenario
    const activeFaults = Object.entries(faults).filter(([, v]) => v > 0)
    if (activeFaults.length > 0) {
      chaosHeaders['X-Crowbar-Fault'] = JSON.stringify(Object.fromEntries(activeFaults))
    }
  }

  const maxAttempts = isIdempotentRead(init) ? Math.max(1, retry.attempts) : 1
  const sleep = retry.sleep ?? defaultSleep

  for (let attempt = 1; ; attempt++) {
    let res: Response
    try {
      res = await fetch(`${API_BASE}${path}`, {
        ...init,
        headers: { ...init?.headers, ...chaosHeaders },
      })
    } catch (err) {
      // Transport-level failure — the request never produced an HTTP response
      // (daemon not ready / connection refused). Retry idempotent reads with
      // bounded backoff; surface the error once attempts are exhausted.
      if (attempt >= maxAttempts) throw err
      await sleep(Math.min(retry.maxDelayMs, retry.baseDelayMs * 2 ** (attempt - 1)))
      continue
    }

    // Status FIRST, body second. `fetch` resolves on 4xx/5xx, so reading the
    // envelope before looking at the status is how an error payload gets
    // mistaken for a result. The daemon does answer errors with the same
    // {success,error,data} envelope, so the body is still worth reading here —
    // deliberately, on the error path, purely to recover its `error` message.
    //
    // An HTTP error status is a real daemon response, not a transport failure —
    // it is terminal (a 404 is meaningful; a 500 is a genuine server error) and
    // must never be retried.
    if (!res.ok) {
      const errorBody = await res.json().catch(() => null)
      throw new ApiError(
        errorBody?.error ?? `${res.status} ${res.statusText}`,
        res.status,
        typeof errorBody?.code === 'string' ? errorBody.code : undefined,
      )
    }
    return res
  }
}

export async function apiFetch<T>(
  path: string,
  init?: RequestInit,
  retry: RetryConfig = DEFAULT_RETRY,
): Promise<T> {
  const res = await apiFetchRaw(path, init, retry)
  // Success with an empty/204/202 body (e.g. WriteMutationOK with no payload, a
  // 204 No Content, or a 202 Accepted for an async hierarchical mutation): the
  // envelope check below would wrongly throw, so treat it as success returning
  // undefined.
  if (res.status === 204 || res.status === 202) {
    return undefined as T
  }
  const body = await res.json().catch(() => null)
  if (body === null) {
    return undefined as T
  }
  if (!body.success) {
    throw new ApiError(body.error ?? `${res.status} ${res.statusText}`, res.status)
  }
  return body.data as T
}

export function fetchProjects(): Promise<Project[]> {
  return apiFetch('/v0/projects')
}

// ---------------------------------------------------------------------------
// Hierarchical READ API (§3/§7).
// ---------------------------------------------------------------------------

export function fetchRepos(projectId: string): Promise<RepoDTO[]> {
  return apiFetch(`/v0/projects/${projectId}/repos`)
}

/**
 * One repo's workspaces, read off its CHAT list.
 *
 * There is no workspace resource to list any more: a worktree is held by a chat,
 * so the git half rides each chat row as `worktree` and this derives the
 * `WorkspaceDTO`s from it. Several rows can carry ONE worktree (a thread carries
 * its parent's workspaceId), so the mapping keeps only the owning row — see
 * `workspaceDTOFromChat` — and the result is still one DTO per worktree.
 */
export async function fetchWorkspaces(projectId: string, repoId: string): Promise<WorkspaceDTO[]> {
  const rows = await apiFetch<RepoChatWireDTO[]>(`/v0/projects/${projectId}/repos/${repoId}/chats`)
  return (rows ?? [])
    .map((row) => workspaceDTOFromChat(row, projectId, repoId))
    .filter((ws): ws is WorkspaceDTO => ws !== null)
}

/**
 * The wire shape of one row GET .../chats/folders returns: a folder-typed
 * domain.Chat rendered through dto.AgentChatDTO (Task 34 — the dedicated
 * `/folders` resource was deleted; folders are Chat rows now). Neither repoId
 * nor projectId travel on it — the URL is the only place either is known —
 * and its display text is `title`, not `name`.
 */
export interface ChatsFolderWireDTO {
  id: string
  parentId: string
  title: string
  order: number
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
  }
}

/**
 * One repo's sidebar folders, in sidebar order.
 *
 * There is no dedicated push channel for this any more — folders lost their
 * own REST+WS resource (backend plan closed; see app-sync-provider.tsx's
 * folders subscription for how a change now reaches the sidebar tree).
 *
 * KNOWN BACKEND LIMITATION, not fixed here: the daemon's `ListInRepo`
 * (api/internal/app/usecases/chat/internal/tree/tree.go) never actually
 * filters by its own `repoID` argument — it filters only by
 * `Type == folder`, so `GET .../chats/folders` returns EVERY folder in the
 * whole daemon, not just this repo's (already self-disclosed in the
 * backend's own container.go comment on the folder push frame's repo
 * scoping). `folderDTOFromWire` below stamps every row it gets back with
 * THIS call's own `repoId`/`projectId` regardless of which repo the row
 * really belongs to, since the wire carries neither — so with more than one
 * repo open, each repo's folder list can end up claiming another repo's
 * folders as its own. There is no correct frontend workaround (the wire row
 * carries no real repoId to filter on); this needs a Go-side fix, out of
 * scope for this task and the closed backend plan.
 */
export async function fetchFolders(projectId: string, repoId: string): Promise<FolderDTO[]> {
  const rows = await apiFetch<ChatsFolderWireDTO[]>(
    `/v0/projects/${projectId}/repos/${repoId}/chats/folders`,
  )
  return (rows ?? []).map((row) => folderDTOFromWire(row, projectId, repoId))
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
    owningChatId: worktree.owningChatId,
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
  worktree?: ChatWorktreeDTO
}

/**
 * A chat-stream frame -> a `WorkspaceDTO`, or null for every frame that is not
 * this chat's own worktree state.
 *
 * The chat sockets carry lifecycle EVENTS, not entity DTOs, and most kinds
 * (`turn_started`, `deleted`, `folder_created`, …) say nothing about a worktree.
 * The owning-row rule is the same one `workspaceDTOFromChat` applies to the
 * list: a thread of the owning chat gets `worktree_state` frames too, and
 * letting one through would write the workspace under the wrong chat's identity.
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
    parentId: row.parentId,
    title: row.title,
    order: row.order ?? 0,
  }
}

/**
 * One repo's chat rows, in sidebar order.
 *
 * Genuinely repo-scoped, unlike `fetchFolders` above: the daemon's
 * `ListChatsInRepo` (api/internal/app/usecases/chat/repo_scope.go) resolves each
 * row's owning repo by walking its ancestry to the nearest provisioned workspace
 * and keeps only the rows that land in THIS repo. A row whose whole ancestry owns
 * no workspace resolves to no repo and is served to none — so a root bubble the
 * spec's §9.1 open question flags as unplaceable never arrives here at all,
 * rather than arriving in every repo's list at once.
 *
 * Stamping this call's own repoId/projectId onto the rows is therefore honest
 * here, where the same line in `fetchFolders` is the documented cross-repo bleed:
 * the server already guaranteed every row belongs to the repo in the URL.
 *
 * Live updates ride `folder-signal.ts`'s per-repo generation, which
 * `use-workspace-agent-chats-stream.ts` bumps on the structural chat frames
 * (created / deleted / title_set / moved) as well as the folder ones — one
 * signal, because chats and folders are ONE aggregate (`domain.Chat`) and one
 * tree.
 */
export async function fetchRepoChats(projectId: string, repoId: string): Promise<ChatDTO[]> {
  const rows = await apiFetch<RepoChatWireDTO[]>(`/v0/projects/${projectId}/repos/${repoId}/chats`)
  return (rows ?? []).map((row) => chatDTOFromWire(row, projectId, repoId))
}

/**
 * One workspace, read through the CHAT that owns its worktree.
 *
 * Callers still hold only a wsId, so the owning chat is resolved from the scope
 * registry the sidebar records — and a missing one throws rather than guessing a
 * URL, the same contract `worktreeVerbBaseForWorkspace` keeps.
 */
export async function fetchWorkspace(
  projectId: string,
  repoId: string,
  wsId: string,
): Promise<WorkspaceDTO> {
  const chatId = getOwningChatId(wsId)
  if (!chatId) throw new Error(`no owning chat recorded for workspace ${wsId}`)
  const row = await apiFetch<RepoChatWireDTO>(
    `/v0/projects/${projectId}/repos/${repoId}/chats/${chatId}`,
  )
  const workspace = row ? workspaceDTOFromChat(row, projectId, repoId) : null
  if (!workspace) throw new Error(`chat ${chatId} holds no worktree for workspace ${wsId}`)
  return workspace
}

// The PROJECT-level home workspace, which is a different entity from a repo's
// default branch: it rides no repo at all, so it appears in NO repo's chat list
// and cannot be derived from one. This route stays as it is.
export function fetchHomeWorkspace(projectId: string): Promise<WorkspaceDTO> {
  return apiFetch(`/v0/projects/${projectId}/home`)
}

// ---------------------------------------------------------------------------
// Hierarchical WRITE API (§3/§7) — every mutation is fire-and-forget: the
// daemon answers 202 Accepted with an empty body and the real entity (with its
// status transitions) arrives over the scoped WS broadcaster. Callers therefore
// await the WS DTO for navigation, never an id from these calls.
// ---------------------------------------------------------------------------

export function postProject(name: string, path: string): Promise<void> {
  return apiFetch('/v0/projects', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, path }),
  })
}

export function postRepo(projectId: string, name: string, path: string): Promise<void> {
  return apiFetch(`/v0/projects/${projectId}/repos`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, path }),
  })
}

// Rename an already-added repo. Answers 204; the updated RepoDTO (new name +
// regenerated avatar) arrives on the repos WS stream and merges into the cache,
// so callers do not update the sidebar store themselves.
export function renameRepo(projectId: string, repoId: string, name: string): Promise<void> {
  return apiFetch(`/v0/projects/${projectId}/repos/${repoId}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  })
}

/**
 * Rename a project. The renamed ProjectDTO arrives on the projects WS stream, so
 * no caller patches its own cache from this — same contract as renameRepo.
 */
export function renameProject(projectId: string, name: string): Promise<void> {
  return apiFetch(`/v0/projects/${projectId}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  })
}

/** Delete a project, and with it every repo and workspace inside. */
export function deleteProject(projectId: string, init?: RequestInit): Promise<void> {
  return apiFetch(`/v0/projects/${projectId}`, { method: 'DELETE', ...init })
}

/**
 * Set (or clear) a workspace's lock.
 *
 * `locked` null is the third state, not a synonym for false: it drops the user's
 * override and hands the question back to the provider, so a branch goes back to
 * being locked exactly when it is protected. Automatic locking is unaffected
 * either way — this only decides whether the user is overruling it.
 */
export function setWorkspaceLock(wsId: string, locked: boolean | null): Promise<void> {
  return apiFetch(`${worktreeVerbBaseForWorkspace(wsId)}/lock`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ locked }),
  })
}

// Open-PR head→base graph for the import dialog's parent hint. Advisory only:
// the import endpoint re-resolves parenting authoritatively server-side.
export function getRepoPullRequests(projectId: string, repoId: string): Promise<PRLink[]> {
  return apiFetch<PRLink[]>(`/v0/projects/${projectId}/repos/${repoId}/pull-requests`)
}

// Batch-import branches as chats holding a worktree each. The daemon PR-parents
// each branch under the row for its open PR's base and creates missing ancestors
// (the whole tree). Resolves on 202-accept; the created rows arrive on the
// workspaces WS stream.
//
// It is a route of its own rather than a loop over POST .../chats, which adopts
// ONE named branch: only this one resolves the PR graph ACROSS a set, creates
// the ancestors a branch is parented under, and falls back to a placeholder row
// for a branch another worktree already holds. Driving it per-branch would drop
// all three silently.
export function importBranches(
  projectId: string,
  repoId: string,
  branches: string[],
): Promise<void> {
  return apiFetch(`/v0/projects/${projectId}/repos/${repoId}/chats/import-batch`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ branches }),
  })
}

// Remove a repository from the project, worktrees and all. The removed RepoDTO
// and its workspaces' tombstones arrive on the entity streams; nothing here
// writes the sidebar tree. This is the one removal the sidebar asks the user to
// confirm — everything under the repo goes with it.
export function deleteRepo(projectId: string, repoId: string, init?: RequestInit): Promise<void> {
  return apiFetch(`/v0/projects/${projectId}/repos/${repoId}`, { method: 'DELETE', ...init })
}

// Rename a worktree's branch, addressed through the CHAT that holds it — a
// worktree verb like lock and merge, on the same repo-scoped chat prefix. The
// daemon renames the git ref and nothing else: the directory is NOT relocated
// any more, so a chat's cwd survives the rename. The updated worktree arrives on
// the chat stream, so — as with renameRepo — callers do not update the sidebar
// store themselves. Answers synchronously: a refusal (name taken, worktree
// locked, branch adopted, not provisioned) arrives as a 409 with a readable
// message while the inline editor is still on screen.
//
// `projectId`/`repoId` stay on the signature for the callers that hold them;
// the URL comes from the recorded scope, which is the only place the owning
// chat is known.
export function renameWorkspaceBranch(
  _projectId: string,
  _repoId: string,
  wsId: string,
  branch: string,
): Promise<void> {
  return apiFetch(`${worktreeVerbBaseForWorkspace(wsId)}/branch`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ branch }),
  })
}

export function fetchPrerequisites(): Promise<Prerequisites> {
  return apiFetch('/v0/system/prerequisites')
}
