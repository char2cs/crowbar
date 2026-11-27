import type { RepoIconSource } from '@/components/layout/repo-icon-mark'
import { useChatTitle } from '@/features/agent/hooks/use-chat-title'
import { useChatIsThread } from '@/features/panes/hooks/use-chat-is-thread'
import { useChatWorking } from '@/features/agent/hooks/use-chat-entity'
import { selectHomeTrees, selectProjects, selectRepos, useSelect } from '@/core/entities/select'
import type { HomeTree } from '@/core/entities/views-from-entities'

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

export interface ChatHeaderModel {
  title: string | null
  working: boolean
  isThread: boolean
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

export function routePath(route: ChatHeaderRoute): string {
  if (route.kind === 'project') return route.name
  const parent =
    route.parentBranch && route.parentBranch !== route.branch ? route.parentBranch : null
  return [route.repoName, parent, route.branch].filter(Boolean).join('/')
}

type ChatPlace = Pick<ChatHeaderModel, 'lead' | 'route'>

export function useChatPlace(chatId: string, wsId: string | null): ChatPlace {
  const repo = useSelect((s) =>
    wsId ? (selectRepos(s).find((r) => r.workspaces.some((w) => w.id === wsId)) ?? null) : null,
  )
  const workspace = repo?.workspaces.find((w) => w.id === wsId) ?? null
  const homeProjectId = useSelect((s) => homeProjectIdOf(selectHomeTrees(s), chatId))
  const project = useSelect((s) =>
    homeProjectId ? (selectProjects(s).find((p) => p.id === homeProjectId) ?? null) : null,
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

export function useChatBranchHeaderModel(chatId: string, wsId: string | null): ChatHeaderModel {
  const title = useChatTitle(chatId)
  const working = useChatWorking(chatId)
  const isThread = useChatIsThread(chatId)
  return { title, working, isThread, ...useChatPlace(chatId, wsId) }
}
