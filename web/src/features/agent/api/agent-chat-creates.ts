import type { LandingChatPresentation } from '@/features/settings/lib/chat-presentation'
import { apiFetch } from '@/lib/api/fetch'
import { operationHeaders } from '@/core/entities/run-operation'
import { chatBase } from '@/features/agent/api/agent-chat-reads'

// ── Writes ──────────────────────────────────────────────────────────
/**
 * Start a chat, optionally UNDER `parentId` — a chat (making the new one a
 * thread of it) or a folder ("new chat in here"). '' is the workspace root.
 *
 * Placement belongs on the create and not in a second call after it. The daemon
 * mints the chat, writes the edge, and only then starts the runner — so a thread
 * has its lineage before its first CLI is spawned and gets its ancestors
 * injected on that very first session. Creating and then placing meant the first
 * turn — the one right after the user asked for a thread — ran with no lineage
 * at all and the agent opened as a stranger. It also left a window in which a
 * failed second call stranded the new chat at the root.
 *
 * A folder INSIDE a chat still yields a thread of that chat: folders carry no
 * turns, so lineage steps straight through them.
 *
 * `surface` is which of the provider's FACES the chat is born on (design spec
 * 2.5). It is not the same thing as `presetChatLandingPresentation`, which
 * only tells the pane where to open: this decides what the daemon actually
 * forks. For a mixed-transport provider (codex) a chat born on 'terminal'
 * gets NO api connection at all: its own PTY is the conversation. Omitted means
 * the provider's own default face, byte-identical to every create before this
 * argument existed.
 *
 * `id` is the chat id the caller minted and already draws; `opId` is the local
 * operation that draws it.
 */
export async function createChat(
  id: string,
  opId: string,
  wsId: string,
  provider: string,
  parentId = '',
  surface?: LandingChatPresentation,
): Promise<string> {
  const res = await apiFetch<{ id: string }>(`${chatBase(wsId)}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', ...operationHeaders(opId) },
    // The repo-scoped mount binds no :wsId (Task 17), so wsId has to travel in
    // the body — the backend's Create falls back to body.workspaceId exactly
    // when the URL carries none. Omitting it anchors the chat to "", and a
    // top-level "" chat has no ancestor to resolve a cwd workspace from, so its
    // runner spawn 404s (agentchat: not found) even though the chat minted.
    body: JSON.stringify({
      id,
      provider,
      parentId,
      workspaceId: wsId,
      ...(surface && { surface }),
    }),
  })
  return res.id
}

/**
 * Create a chat AND mint a fresh workspace for it to own, atomically (model
 * spec §4.1's "own worktree" create; backend Task 7,
 * `chat/handlers/chats.go`'s `Create`). Used by the sidebar's "create
 * workspace" affordance (space-content-actions.ts's `handleCreate`) in place
 * of the old two-step `postWorkspace` (a bare, chat-less workspace) plus a
 * later, separate chat create — this is ONE request, so the row that results
 * already has its first conversation running.
 *
 * Unlike `createChat`, this names NO workspace at all — there is none to name
 * yet — so it is built straight off project+repo rather than through
 * `chatBase`'s workspace-scope resolution, which requires an EXISTING
 * workspace to resolve a scope from. `workspaceId` is omitted from the body
 * entirely: the backend's `ownWorktree` only takes effect when the request
 * names no workspace, and an omitted key is the same "" its Go struct would
 * bind anyway.
 *
 * `branch` is the name the user typed in the sidebar's inline create input
 * (2026-09-09) — omitted (server-generated name) unless the caller has one.
 * The reply names the new chat and the workspace it owns.
 */
export function createChatWithOwnWorktree(
  id: string,
  opId: string,
  projectId: string,
  repoId: string,
  provider: string,
  parentId = '',
  branch = '',
): Promise<{ id: string; workspaceId: string }> {
  const p = encodeURIComponent(projectId)
  const r = encodeURIComponent(repoId)
  return apiFetch<{ id: string; workspaceId: string }>(`/v0/projects/${p}/repos/${r}/chats`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', ...operationHeaders(opId) },
    body: JSON.stringify({
      id,
      provider,
      parentId,
      ownWorktree: true,
      ...(branch && { branch }),
    }),
  })
}
