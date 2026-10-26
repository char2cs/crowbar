import type { RepoIconSource } from '@/components/layout/repo-icon-mark'
import { useChatTitle } from '@/features/agent/hooks/use-chat-title'
import { useChatIsThread } from '@/features/panes/hooks/use-chat-is-thread'
import { useWorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { dataOf } from '@/lib/loadable'
import { useHomeTreeStore, type HomeTree } from '@/lib/store/home-tree'
import { useProjectDataStore } from '@/lib/store/projects'
import { useSidebarStore } from '@/lib/store/sidebar'

/** Where a chat runs, as its header names it. */
export type ChatHeaderRoute =
  | { kind: 'project'; name: string }
  | {
      kind: 'branch'
      repoName?: string
      parentBranch?: string
      branch: string
      added: number
      deleted: number
    }

/**
 * Everything the chat header draws, as plain data: the pane header reads it from the stores, the
 * phone's file drawer (a page with no stores) receives it as a snapshot. `title` null is the
 * store's gap, not an untitled chat.
 */
export interface ChatHeaderModel {
  title: string | null
  working: boolean
  isThread: boolean
  /** Null: neither a repo nor a project to show, so the kind mark takes the icon's place. */
  lead:
    | { kind: 'repo'; repo: RepoIconSource }
    | {
        kind: 'project'
        project: { id: string; name: string; avatarEmoji?: string; avatarUrl?: string }
      }
    | null
  route: ChatHeaderRoute | null
}

function homeProjectIdOf(trees: Record<string, HomeTree>, chatId: string): string | null {
  for (const projectId of Object.keys(trees)) {
    if (trees[projectId].chats.some((c) => c.id === chatId)) return projectId
  }
  return null
}

/** `repo/parent/branch` (or the project's name) as one line of text, for surfaces without room for the segmented header. */
export function routePath(route: ChatHeaderRoute): string {
  if (route.kind === 'project') return route.name
  const parent =
    route.parentBranch && route.parentBranch !== route.branch ? route.parentBranch : null
  return [route.repoName, parent, route.branch].filter(Boolean).join('/')
}

type ChatPlace = Pick<ChatHeaderModel, 'lead' | 'route'>

/** Where a chat runs, resolved from the sidebar and home trees; `route` null when no visible project holds it. */
export function useChatPlace(chatId: string, wsId: string | null): ChatPlace {
  const repo = useSidebarStore((s) =>
    wsId ? (s.repos.find((r) => r.workspaces.some((w) => w.id === wsId)) ?? null) : null,
  )
  const workspace = repo?.workspaces.find((w) => w.id === wsId) ?? null
  // A project-home chat rides no repo (home-tree.ts), so the chat's own
  // project id comes from the home tree that lists it, never from `wsId`.
  const homeProjectId = useHomeTreeStore((s) => homeProjectIdOf(s.trees, chatId))
  const project = useProjectDataStore((s) =>
    homeProjectId ? (dataOf(s.data)?.find((p) => p.id === homeProjectId) ?? null) : null,
  )

  if (workspace?.branch && repo) {
    return {
      lead: {
        kind: 'repo',
        repo: {
          name: repo.name,
          avatarURL: repo.avatarURL,
          avatarLabel: repo.avatarLabel,
          avatarColor: repo.avatarColor,
        },
      },
      route: {
        kind: 'branch',
        repoName: repo.name,
        parentBranch: workspace.parentBranch,
        branch: workspace.branch,
        added: workspace.added ?? 0,
        deleted: workspace.deleted ?? 0,
      },
    }
  }
  return {
    lead: project
      ? {
          kind: 'project',
          project: {
            id: project.id,
            name: project.name,
            avatarEmoji: project.avatarEmoji,
            avatarUrl: project.avatarUrl,
          },
        }
      : null,
    route: project ? { kind: 'project', name: project.name } : null,
  }
}

/** The chat header's content, resolved from the workspace store in context and the sidebar. */
export function useChatBranchHeaderModel(chatId: string, wsId: string | null): ChatHeaderModel {
  const title = useChatTitle(chatId)
  const working = useWorkspaceStoreContext((s) => s.agentChats.working[chatId] ?? false)
  // A thread owns no branch (it runs on its parent's), and inherits its parent's `wsId`.
  const isThread = useChatIsThread(chatId)
  return { title, working, isThread, ...useChatPlace(chatId, wsId) }
}
